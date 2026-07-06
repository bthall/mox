package tmux

// Tmux is the surface area used by the session builder and manager. It is
// satisfied by *Client and by test fakes. The interface is consumer-defined:
// it lists exactly what callers need, no more.
type Tmux interface {
	SessionExists(name string) (bool, error)
	ListSessions() ([]string, error)
	ListSessionsDetailed() ([]SessionDetail, error)
	CreateSession(name, startDir, firstWindow string) error
	KillSession(name string) error
	AttachSession(name string) error

	CreateWindow(session, name, startDir string) (string, error)
	RenameWindow(target, newName string) error
	SelectWindowByID(windowID string) error

	FirstWindowID(session string) (string, error)
	FirstPaneID(windowID string) (string, error)

	SplitPane(target string, dir SplitDirection, sizePercent int, startDir string) (string, error)
	SendKeys(target string, commands []string) error
	SetPaneTitle(target, title string) error
	SetSessionOption(session, option, value string) error
	SetWindowOption(windowTarget, option, value string) error
	SelectLayout(windowTarget, layoutName string) error
	SetHook(session, event, command string) error
}

// compile-time assertion that *Client satisfies Tmux.
var _ Tmux = (*Client)(nil)
