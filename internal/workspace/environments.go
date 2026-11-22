package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Environment struct{ Name, Kind, Target, Root, Binary string }
type Environments struct{ Profiles, Recent []Environment }

func (e Environment) Key() string {
	root := path.Clean(e.Root)
	if e.Kind == "" {
		root = filepath.Clean(e.Root)
	}
	data, _ := json.Marshal([]string{e.Kind, e.Target, root})
	return string(data)
}
func (e Environment) Label() string {
	if e.Name != "" {
		return e.Name
	}
	if e.Kind == "" {
		return e.Root
	}
	target := e.Target
	if target == "" && e.Kind == "wsl" {
		target = "默认发行版"
	}
	return strings.ToUpper(e.Kind) + ": " + target + " · " + e.Root
}
func (s *Environments) Remember(e Environment) {
	recent := []Environment{e}
	for _, old := range s.Recent {
		if old.Key() != e.Key() {
			recent = append(recent, old)
		}
		if len(recent) >= 20 {
			break
		}
	}
	s.Recent = recent
}
func (s *Environments) SaveProfile(e Environment) error {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return fmt.Errorf("请输入配置名称")
	}
	for i, old := range s.Profiles {
		if old.Name == e.Name {
			s.Profiles[i] = e
			return nil
		}
	}
	s.Profiles = append(s.Profiles, e)
	return nil
}
func (s *Environments) RemoveProfile(name string) {
	for i, e := range s.Profiles {
		if e.Name == name {
			s.Profiles = append(s.Profiles[:i], s.Profiles[i+1:]...)
			return
		}
	}
}
func LoadEnvironments(file string) (Environments, error) {
	var s Environments
	data, err := os.ReadFile(file)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	if len(s.Recent) > 20 {
		s.Recent = s.Recent[:20]
	}
	return s, err
}
func SaveEnvironments(file string, s Environments) error { return saveJSON(file, s) }
