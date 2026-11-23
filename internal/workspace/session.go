package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type OpenFile struct {
	Path   string
	Cursor int
}
type Session struct {
	ConnectionKind, Target, Binary string
	Port                           int
	Root                           string
	Files                          []OpenFile
	Active                         string
	SideWidth, EditorShare         float32
	Sidebar, Terminal, Dark, Wrap  bool
	RightPanel                     bool
	Section                        string
	FontSize                       float32
	Recent                         []string
	// The editor's own settings: what a zero says is the default of the
	// language, or of the editor.
	TabSize     int
	Indent      int
	Whitespace  int
	LineHeight  float32
	NoGuides    bool
	NoAutoClose bool
	NoNumbers   bool
	// Theme is the theme of the window, and Icons the theme of the marks
	// of files; Dark is what a session from before them has.
	Theme string
	Icons string
}

func SessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vael", "session.json"), nil
}
func LoadSession(path string) (Session, error) {
	var s Session
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}
func SaveSession(path string, s Session) error {
	return saveJSON(path, s)
}
func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
