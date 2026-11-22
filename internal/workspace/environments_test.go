package workspace

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnvironmentHistoryDeduplicatesAndKeepsMostRecent(t *testing.T) {
	var s Environments
	local := Environment{Root: filepath.Join(t.TempDir(), "project")}
	remote := Environment{Kind: "ssh", Target: "dev", Root: local.Root, Binary: "vael-core"}
	s.Remember(local)
	s.Remember(remote)
	local.Name = "Local project"
	s.Remember(local)
	if len(s.Recent) != 2 || s.Recent[0].Name != local.Name || s.Recent[1].Kind != "ssh" {
		t.Fatalf("%+v", s.Recent)
	}
	for i := range 30 {
		s.Remember(Environment{Kind: "docker", Target: fmt.Sprintf("container-%d", i), Root: "/project"})
	}
	if len(s.Recent) != 20 || s.Recent[0].Target != "container-29" {
		t.Fatalf("%+v", s.Recent)
	}
}
func TestProfilesPersistReplaceAndRemove(t *testing.T) {
	var s Environments
	if s.SaveProfile(Environment{Name: " "}) == nil {
		t.Fatal("blank name accepted")
	}
	profile := Environment{Name: " Development ", Kind: "wsl", Target: "Ubuntu", Root: "/home/dev", Binary: "/usr/local/bin/vael-core"}
	if err := s.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	profile.Name = "Development"
	profile.Root = "/home/dev/new"
	_ = s.SaveProfile(profile)
	if len(s.Profiles) != 1 || s.Profiles[0].Root != profile.Root {
		t.Fatal("profile not replaced")
	}
	s.Remember(profile)
	file := filepath.Join(t.TempDir(), "config", "environments.json")
	if err := SaveEnvironments(file, s); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadEnvironments(file)
	if err != nil || !reflect.DeepEqual(s, saved) {
		t.Fatalf("%+v %v", saved, err)
	}
	saved.RemoveProfile("Development")
	if len(saved.Profiles) != 0 || len(saved.Recent) != 1 {
		t.Fatal("removal affected history")
	}
}
