package tools

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

var dbusConn *dbus.Conn

func init() {
	// Connect to session D-Bus (lazy initialization for availability checks)
	var err error
	dbusConn, err = dbus.ConnectSessionBus()
	if err != nil {
		// D-Bus unavailable; all methods will fail gracefully
		dbusConn = nil
	}
}

// PlasmaStatus returns availability and capabilities of Plasma integration.
func PlasmaStatus() map[string]any {
	if dbusConn == nil {
		return map[string]any{"available": false, "reason": "D-Bus session not available"}
	}
	return map[string]any{
		"available": true,
		"service":   "org.kde.Plasma",
		"methods":   []string{"SetTheme", "SetWallpaper", "GetPanelConfig", "SetWidgetConfig"},
	}
}

// SetTheme changes the KDE Plasma theme via D-Bus.
// theme: name of the theme (e.g., "Breeze", "Oxygen", custom theme name).
func SetTheme(theme string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// Call org.kde.Plasma.DesktopShell /org/kde/Plasma/DesktopShell setTheme(theme_name)
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma/DesktopShell")
	call := obj.Call("org.kde.Plasma.DesktopShell.SetTheme", 0, theme)
	if call.Err != nil {
		return fmt.Errorf("failed to set Plasma theme: %w", call.Err)
	}
	return nil
}

// SetWallpaper changes the KDE Plasma desktop wallpaper via D-Bus.
// imagePath: absolute path to wallpaper image file.
func SetWallpaper(imagePath string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// Call org.kde.Plasma.DesktopShell /org/kde/Plasma/DesktopShell setWallpaper(path)
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma/DesktopShell")
	call := obj.Call("org.kde.Plasma.DesktopShell.SetWallpaper", 0, imagePath)
	if call.Err != nil {
		return fmt.Errorf("failed to set Plasma wallpaper: %w", call.Err)
	}
	return nil
}

// GetPanelConfig retrieves current KDE panel configuration via D-Bus.
// Returns map with panel name, position, height, and widgets.
func GetPanelConfig() (map[string]any, error) {
	if dbusConn == nil {
		return nil, fmt.Errorf("D-Bus session not available")
	}

	// Call org.kde.Plasma /org/kde/Plasma PanelConfig()
	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma")
	call := obj.Call("org.kde.Plasma.GetPanelConfig", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to get panel config: %w", call.Err)
	}

	var config map[string]any
	if err := call.Store(&config); err != nil {
		return nil, fmt.Errorf("failed to parse panel config: %w", err)
	}
	return config, nil
}

// SetWidgetConfig updates configuration for a specific Plasma widget via D-Bus.
// widgetName: name of the widget (e.g., "taskmanager", "systemtray", "clock").
// config: map of configuration key-value pairs.
func SetWidgetConfig(widgetName string, config map[string]string) error {
	if dbusConn == nil {
		return fmt.Errorf("D-Bus session not available")
	}

	// Call org.kde.Plasma /org/kde/Plasma/Widgets/{widgetName} SetConfig(config_dict)
	path := dbus.ObjectPath(fmt.Sprintf("/org/kde/Plasma/Widgets/%s", widgetName))
	obj := dbusConn.Object("org.kde.Plasma", path)
	call := obj.Call("org.kde.Plasma.Widgets.SetConfig", 0, config)
	if call.Err != nil {
		return fmt.Errorf("failed to set widget config for %s: %w", widgetName, call.Err)
	}
	return nil
}

// ListPanelWidgets returns list of all active widgets on panels via D-Bus.
func ListPanelWidgets() ([]map[string]any, error) {
	if dbusConn == nil {
		return nil, fmt.Errorf("D-Bus session not available")
	}

	obj := dbusConn.Object("org.kde.Plasma", "/org/kde/Plasma")
	call := obj.Call("org.kde.Plasma.ListWidgets", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("failed to list widgets: %w", call.Err)
	}

	var widgets []map[string]any
	if err := call.Store(&widgets); err != nil {
		return nil, fmt.Errorf("failed to parse widgets list: %w", err)
	}
	return widgets, nil
}
