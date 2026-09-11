package annotation

type Case struct {
	TaskID           string    `json:"taskId"`
	ProjectID        string    `json:"projectId"`
	TaskName         string    `json:"taskName"`
	SourcePath       string    `json:"sourcePath"`
	InitialSHA       string    `json:"initialSha"`
	SnapshotURL      string    `json:"snapshotUrl"`
	ContainerID      string    `json:"containerId"`
	ContainerName    string    `json:"containerName"`
	WorkspacePath    string    `json:"workspacePath"`
	RepoRelativePath string    `json:"repoRelativePath"`
	SessionID        string    `json:"sessionId"`
	TracePath        string    `json:"tracePath"`
	Completed        bool      `json:"completed"`
	Rounds           []Round   `json:"rounds"`
	Captures         []Capture `json:"captures"`
	Revision         int       `json:"revision"`
	UpdatedAt        int64     `json:"updatedAt"`
}

type Round struct {
	PromptID     string       `json:"promptId"`
	SessionID    string       `json:"sessionId"`
	Prompt       string       `json:"prompt"`
	Order        int          `json:"order"`
	Status       string       `json:"status"`
	Reason       string       `json:"reason"`
	EvidenceHash string       `json:"evidenceHash"`
	SourceStart  int          `json:"sourceStart"`
	SourceEnd    int          `json:"sourceEnd"`
	Version      string       `json:"version"`
	Cwd          string       `json:"cwd"`
	Attachments  []string     `json:"attachments"`
	CaptureID    string       `json:"captureId"`
	Evaluations  []Evaluation `json:"evaluations"`
}

type Capture struct {
	ID        string `json:"id"`
	Dir       string `json:"dir"`
	TracePath string `json:"tracePath"`
	CodePath  string `json:"codePath"`
	Hash      string `json:"hash"`
	TraceHash string `json:"traceHash"`
	CreatedAt int64  `json:"createdAt"`
}

type Evaluation struct {
	ID             string    `json:"id"`
	CreatedAt      int64     `json:"createdAt"`
	SkillHash      string    `json:"skillHash"`
	Model          string    `json:"model"`
	EvidenceHash   string    `json:"evidenceHash"`
	SourceHash     string    `json:"sourceHash"`
	ReviewPath     string    `json:"reviewPath"`
	ReviewHash     string    `json:"reviewHash"`
	Status         string    `json:"status"`
	Scores         [5]*int   `json:"scores"`
	Descriptions   [5]string `json:"descriptions"`
	TaskType       string    `json:"taskType"`
	Difficulty     string    `json:"difficulty"`
	Language       string    `json:"language"`
	Environment    string    `json:"environment"`
	HarnessVersion string    `json:"harnessVersion"`
	OS             string    `json:"os"`
	Evidence       []string  `json:"evidence"`
	Missing        []string  `json:"missing"`
	Issues         []Issue   `json:"issues"`
	NextPrompt     string    `json:"nextPrompt"`
	NextPromptType string    `json:"nextPromptType"`
}

type Issue struct {
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Kind        string `json:"kind"`
}

type Report struct {
	Tasks  int      `json:"tasks"`
	Rounds int      `json:"rounds"`
	Ready  int      `json:"ready"`
	Issues []string `json:"issues"`
}
