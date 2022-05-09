package keystone

// GetLabels -
func GetLabels(name string) map[string]string {
	return map[string]string{"owner": "keystone-operator", "cr": "keystone-" + name, "app": "keystone"}
}
