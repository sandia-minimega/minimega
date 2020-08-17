package type

type Logs struct {
	Timestamp string `json:"timestamp"`
	Source string `json:"source"`
	Level string `json:"level"`
	Log string `json:"log"`
}