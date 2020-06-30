package priv

const CommandAction = "command"

type CommandRequest struct {
	Command string
	Args    []string
	Stdin   string
}

type CommandResponse struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

const WritePfConfAction = "writePfConf"

type WritePfConfRequest struct {
	Content string
}

type WritePfConfResponse struct {
	Error string
}
