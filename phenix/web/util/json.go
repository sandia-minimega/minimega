package util

func WithRoot(key string, obj interface{}) map[string]interface{} {
	return map[string]interface{}{key: obj}
}
