package types

type Captures struct {
	Captures []Capture `json:"captures"`
}

type Capture struct {
	VM        string `json:"vm"`
	Interface int    `json:"interface"`
	Filepath  string `json:"filepath"`
}
