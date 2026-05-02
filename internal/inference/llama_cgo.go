//go:build cgo

package inference

/*
#include <stdlib.h>
#include "llama.h"
*/
import "C"
import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

var llamaBackendOnce sync.Once

func ensureBackendInit() {
	llamaBackendOnce.Do(func() {
		C.llama_backend_init()
	})
}

// LlamaModel wraps the C model pointer.
type LlamaModel struct {
	ptr *C.struct_llama_model
}

// LlamaContext wraps the C context pointer.
type LlamaContext struct {
	ptr *C.struct_llama_context
}

func LoadModel(path string, nGPULayers int) (*LlamaModel, error) {
	ensureBackendInit()

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	params := C.llama_model_default_params()
	params.n_gpu_layers = C.int(nGPULayers)
	params.use_mmap = true
	params.use_mlock = false

	model := C.llama_model_load_from_file(cpath, params)
	if model == nil {
		return nil, fmt.Errorf("failed to load model from %s", path)
	}
	return &LlamaModel{ptr: model}, nil
}

func (m *LlamaModel) NewContext(nCtx int) (*LlamaContext, error) {
	params := C.llama_context_default_params()
	params.n_ctx = C.uint32_t(nCtx)
	params.n_batch = C.uint32_t(max(256, nCtx/2))
	params.n_ubatch = C.uint32_t(max(64, min(512, nCtx)))
	params.n_threads = C.int32_t(0)
	params.n_threads_batch = C.int32_t(0)
	params.no_perf = false

	ctx := C.llama_init_from_model(m.ptr, params)
	if ctx == nil {
		return nil, fmt.Errorf("failed to create context")
	}
	return &LlamaContext{ptr: ctx}, nil
}

func (m *LlamaModel) vocab() *C.struct_llama_vocab {
	return C.llama_model_get_vocab(m.ptr)
}

func (m *LlamaModel) Tokenize(text string, addSpecial bool) ([]int32, error) {
	vocab := m.vocab()
	if vocab == nil {
		return nil, fmt.Errorf("model vocab is unavailable")
	}

	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))

	addSpecialC := C.bool(addSpecial)
	parseSpecialC := C.bool(true)
	n := C.llama_tokenize(vocab, ctext, C.int32_t(len(text)), (*C.llama_token)(nil), 0, addSpecialC, parseSpecialC)
	if n == C.INT32_MIN {
		return nil, fmt.Errorf("tokenization overflow")
	}
	if n < 0 {
		n = -n
	}
	if n == 0 {
		return []int32{}, nil
	}

	tokens := make([]C.llama_token, int(n))
	r := C.llama_tokenize(vocab, ctext, C.int32_t(len(text)), &tokens[0], C.int32_t(len(tokens)), addSpecialC, parseSpecialC)
	if r < 0 {
		return nil, fmt.Errorf("tokenization failed")
	}
	result := make([]int32, int(r))
	for i := range result {
		result[i] = int32(tokens[i])
	}
	return result, nil
}

func (m *LlamaModel) TokenToText(token int32) (string, error) {
	vocab := m.vocab()
	if vocab == nil {
		return "", fmt.Errorf("model vocab is unavailable")
	}
	buf := make([]byte, 256)
	n := C.llama_token_to_piece(vocab, C.llama_token(token), (*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)), 0, true)
	if n < 0 {
		size := int(-n)
		buf = make([]byte, size)
		n = C.llama_token_to_piece(vocab, C.llama_token(token), (*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)), 0, true)
		if n < 0 {
			return "", fmt.Errorf("failed to convert token to text")
		}
	}
	return string(buf[:n]), nil
}

func (m *LlamaModel) Detokenize(tokens []int32) (string, error) {
	if len(tokens) == 0 {
		return "", nil
	}
	var builder strings.Builder
	for _, token := range tokens {
		piece, err := m.TokenToText(token)
		if err != nil {
			return "", err
		}
		builder.WriteString(piece)
	}
	return builder.String(), nil
}

func (ctx *LlamaContext) Eval(tokens []int32, pastTokens int) error {
	if len(tokens) == 0 {
		return nil
	}
	cTokens := make([]C.llama_token, len(tokens))
	for i, token := range tokens {
		cTokens[i] = C.llama_token(token)
	}
	batch := C.llama_batch_get_one(&cTokens[0], C.int32_t(len(cTokens)))
	defer C.llama_batch_free(batch)

	ret := C.llama_decode(ctx.ptr, batch)
	if ret != 0 {
		return fmt.Errorf("decode failed with code %d", int(ret))
	}
	_ = pastTokens
	return nil
}

func (m *LlamaModel) Generate(ctx *LlamaContext, prompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 128
	}

	vocab := m.vocab()
	if vocab == nil {
		return "", fmt.Errorf("model vocab is unavailable")
	}

	promptTokens, err := m.Tokenize(prompt, bool(C.llama_vocab_get_add_bos(vocab)))
	if err != nil {
		return "", err
	}
	if len(promptTokens) == 0 && bool(C.llama_vocab_get_add_bos(vocab)) {
		promptTokens = append(promptTokens, int32(C.llama_vocab_bos(vocab)))
	}
	if err := ctx.Eval(promptTokens, 0); err != nil {
		return "", err
	}

	chainParams := C.llama_sampler_chain_default_params()
	chainParams.no_perf = true
	sampler := C.llama_sampler_chain_init(chainParams)
	if sampler == nil {
		return "", fmt.Errorf("failed to initialize sampler chain")
	}
	defer C.llama_sampler_free(sampler)
	greedy := C.llama_sampler_init_greedy()
	if greedy != nil {
		C.llama_sampler_chain_add(sampler, greedy)
	}

	generated := make([]int32, 0, maxTokens)
	for i := 0; i < maxTokens; i++ {
		token := int32(C.llama_sampler_sample(sampler, ctx.ptr, -1))
		if C.llama_vocab_is_eog(vocab, C.llama_token(token)) {
			break
		}
		generated = append(generated, token)
		if err := ctx.Eval([]int32{token}, len(promptTokens)+i); err != nil {
			return "", err
		}
		C.llama_sampler_accept(sampler, C.llama_token(token))
	}

	return m.Detokenize(generated)
}

func (ctx *LlamaContext) Free() {
	if ctx.ptr != nil {
		C.llama_free(ctx.ptr)
		ctx.ptr = nil
	}
}

func (m *LlamaModel) Free() {
	if m.ptr != nil {
		C.llama_model_free(m.ptr)
		m.ptr = nil
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
