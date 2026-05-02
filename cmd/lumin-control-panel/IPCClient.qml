import QtQuick
import QtCore

// IPC client for communicating with lumin-engine daemon via Unix socket
QtObject {
    id: root

    // Properties for UI binding
    property string daemonStatus: "Connecting..."
    property string hardwareInfo: ""
    property int contextSize: 0
    property string currentModel: "None"
    property var availableModels: []
    property var permissions: []
    property string auditLog: ""
    property string generationResult: ""

    // Signals
    signal statusChanged()
    signal modelLoaded(string modelName)
    signal error(string errorMessage)

    // Socket connection
    property var socket: null

    function connect() {
        // Connect to /run/lumin/engine.sock via Unix domain socket
        socket = new WebSocket("unix:///run/lumin/engine.sock")
        socket.onopen = onSocketOpen
        socket.onmessage = onSocketMessage
        socket.onerror = onSocketError
        socket.onclose = onSocketClose
    }

    function onSocketOpen() {
        daemonStatus = "Connected"
        statusChanged()
        requestStatus()
    }

    function onSocketMessage(event) {
        try {
            let msg = JSON.parse(event.data)
            handleMessage(msg)
        } catch(e) {
            error("Failed to parse message: " + e)
        }
    }

    function onSocketError(event) {
        daemonStatus = "Error"
        error("Socket error: " + event.message)
        statusChanged()
    }

    function onSocketClose() {
        daemonStatus = "Disconnected"
        statusChanged()
    }

    function sendMessage(method, params = {}) {
        if (!socket) return
        let msg = {
            jsonrpc: "2.0",
            method: method,
            params: params,
            id: Math.random()
        }
        socket.send(JSON.stringify(msg))
    }

    function handleMessage(msg) {
        if (!msg.result) return

        switch(msg.method) {
        case "health":
            daemonStatus = msg.result.status
            hardwareInfo = msg.result.hardware?.gpu_name || "Unknown"
            contextSize = msg.result.max_context || 2048
            currentModel = msg.result.model?.name || "None"
            statusChanged()
            break
        case "models_list":
            availableModels = msg.result.models || []
            statusChanged()
            break
        case "permissions":
            permissions = msg.result.permissions || []
            statusChanged()
            break
        case "audit_log":
            auditLog = msg.result.log || ""
            statusChanged()
            break
        case "generate":
            generationResult = msg.result.output || ""
            statusChanged()
            break
        }
    }

    function requestStatus() {
        sendMessage("Health")
        sendMessage("ListModels")
        sendMessage("GetPermissions")
    }

    function loadModel(modelPath) {
        sendMessage("LoadModel", {path: modelPath})
        modelLoaded(modelPath)
    }

    function generate(prompt, maxTokens, temperature, topP) {
        sendMessage("Generate", {
            prompt: prompt,
            max_tokens: maxTokens,
            temperature: temperature,
            top_p: topP
        })
    }

    function setPermission(permissionId, granted) {
        sendMessage("SetPermission", {
            permission_id: permissionId,
            granted: granted
        })
    }

    function disconnect() {
        if (socket) socket.close()
    }

    Component.onDestruction: disconnect()
}
