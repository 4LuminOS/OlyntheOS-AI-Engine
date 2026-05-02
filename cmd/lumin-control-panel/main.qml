import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import org.kde.kirigami as Kirigami

Kirigami.ApplicationWindow {
    id: root
    title: "LuminAI Control Panel"
    width: 900
    height: 700

    // IPC connection to lumin-engine daemon
    IPCClient {
        id: ipcClient
        onStatusChanged: updateUI()
        onModelLoaded: {
            messageBox.text = "Model loaded successfully: " + modelName
            messageBox.visible = true
        }
        onError: {
            errorBox.text = "Error: " + errorMessage
            errorBox.visible = true
        }
    }

    pageStack.initialPage: Kirigami.ScrollablePage {
        title: "LuminAI Engine"
        topPadding: Kirigami.Units.largeSpacing
        leftPadding: Kirigami.Units.largeSpacing
        rightPadding: Kirigami.Units.largeSpacing
        bottomPadding: Kirigami.Units.largeSpacing

        ColumnLayout {
            width: parent.width
            spacing: Kirigami.Units.mediumSpacing

            // Status section
            Kirigami.FormLayout {
                Layout.fillWidth: true

                TextField {
                    Kirigami.FormData.label: "Status:"
                    readOnly: true
                    text: ipcClient.daemonStatus
                }

                TextField {
                    Kirigami.FormData.label: "Hardware:"
                    readOnly: true
                    text: ipcClient.hardwareInfo
                }

                TextField {
                    Kirigami.FormData.label: "Context Window:"
                    readOnly: true
                    text: ipcClient.contextSize + " tokens"
                }
            }

            Kirigami.Separator {
                Layout.fillWidth: true
            }

            // Model management section
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Kirigami.Units.smallSpacing

                Label {
                    text: "Model Management"
                    font.bold: true
                    font.pixelSize: Kirigami.Units.gridUnit * 1.3
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.mediumSpacing

                    ComboBox {
                        id: modelSelector
                        model: ipcClient.availableModels
                        Layout.fillWidth: true
                        textRole: "display"
                    }

                    Button {
                        text: "Load"
                        icon.name: "folder-open"
                        onClicked: ipcClient.loadModel(modelSelector.currentText)
                    }
                }

                Label {
                    text: "Current Model: " + ipcClient.currentModel
                    font.pixelSize: Kirigami.Units.gridUnit
                    color: Kirigami.Theme.textColor
                }
            }

            Kirigami.Separator {
                Layout.fillWidth: true
            }

            // Generate section
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Kirigami.Units.smallSpacing

                Label {
                    text: "Generate Text"
                    font.bold: true
                    font.pixelSize: Kirigami.Units.gridUnit * 1.3
                }

                TextArea {
                    id: promptInput
                    placeholderText: "Enter prompt here..."
                    Layout.fillWidth: true
                    Layout.minimumHeight: Kirigami.Units.gridUnit * 5
                }

                Kirigami.FormLayout {
                    Layout.fillWidth: true

                    SpinBox {
                        Kirigami.FormData.label: "Max Tokens:"
                        from: 1
                        to: 4096
                        value: 256
                        id: maxTokensSpinBox
                    }

                    Slider {
                        Kirigami.FormData.label: "Temperature:"
                        from: 0.0
                        to: 2.0
                        stepSize: 0.1
                        value: 0.7
                        id: temperatureSlider
                        Layout.fillWidth: true
                    }

                    Slider {
                        Kirigami.FormData.label: "Top-P:"
                        from: 0.0
                        to: 1.0
                        stepSize: 0.05
                        value: 0.9
                        id: topPSlider
                        Layout.fillWidth: true
                    }
                }

                Button {
                    text: "Generate"
                    icon.name: "play"
                    Layout.fillWidth: true
                    onClicked: {
                        ipcClient.generate(
                            promptInput.text,
                            maxTokensSpinBox.value,
                            temperatureSlider.value,
                            topPSlider.value
                        )
                    }
                }
            }

            TextArea {
                id: generationOutput
                readOnly: true
                placeholderText: "Generation output will appear here..."
                Layout.fillWidth: true
                Layout.fillHeight: true
                text: ipcClient.generationResult
            }

            // Permission editor section
            RowLayout {
                Layout.fillWidth: true
                spacing: Kirigami.Units.mediumSpacing

                Button {
                    text: "Edit Permissions"
                    icon.name: "system-lock-screen"
                    onClicked: permissionsDialog.open()
                }

                Button {
                    text: "View Audit Log"
                    icon.name: "document-properties"
                    onClicked: auditLogDialog.open()
                }
            }
        }
    }

    // Message and error dialogs
    Kirigami.PromptDialog {
        id: messageBox
        title: "Success"
        standardButtons: Dialog.Ok
    }

    Kirigami.PromptDialog {
        id: errorBox
        title: "Error"
        standardButtons: Dialog.Ok
    }

    // Permissions editor dialog
    Dialog {
        id: permissionsDialog
        title: "Permission Settings"
        width: 600
        height: 500

        ColumnLayout {
            width: parent.width
            spacing: Kirigami.Units.mediumSpacing

            Label {
                text: "Grant or revoke model capabilities"
            }

            Repeater {
                model: ipcClient.permissions
                delegate: CheckBox {
                    text: modelData.name
                    checked: modelData.granted
                    onToggled: ipcClient.setPermission(modelData.id, checked)
                }
            }
        }

        standardButtons: Dialog.Ok | Dialog.Cancel
    }

    // Audit log viewer
    Dialog {
        id: auditLogDialog
        title: "Audit Log"
        width: 800
        height: 600

        TextArea {
            width: parent.width
            height: parent.height
            readOnly: true
            text: ipcClient.auditLog
        }

        standardButtons: Dialog.Ok
    }

    Component.onCompleted: {
        ipcClient.connect()
        updateUI()
    }

    function updateUI() {
        generationOutput.text = ipcClient.generationResult
    }
}
