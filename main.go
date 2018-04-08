package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func main() {
	log.Print("Hello, world!")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	kumara := Kumara{watcher, nil}

	done := make(chan bool)
	go kumara.Watch()
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	err = filepath.Walk(dir, kumara.Visit)
	if err != nil {
		log.Fatal(err)
	}
	kumara.Build()
	err = kumara.Restart()
	if err != nil {
		log.Fatal(err)
	}

	<-done
}

type Kumara struct {
	watcher *fsnotify.Watcher
	command *exec.Cmd
}

func (k Kumara) Visit(path string, info os.FileInfo, err error) error {
	if err != nil {
		log.Print(err)
		return filepath.SkipDir
	}

	if !info.IsDir() {
		return nil
	}

	if strings.HasPrefix(info.Name(), ".") {
		return filepath.SkipDir
	}

	k.Add(path)
	return nil
}

func (k Kumara) Add(path string) {
	log.Print("Add: ", path)
	err := k.watcher.Add(path)
	if err != nil {
		log.Print("Error: ", err)
	}
}

func (k Kumara) Build() error {
	log.Print("Building...")
	args := []string{"go", "build", "-o", "kumara-bin"}
	command := exec.Command(args[0], args[1:]...)
	output, err := command.CombinedOutput()

	if !command.ProcessState.Success() {
		log.Print(string(output))
	}
	return err
}

func (k Kumara) Restart() error {

	if k.command != nil && k.command.Process != nil {
		log.Print("Stopping process")
		err := k.command.Process.Kill()
		if err != nil {
			log.Print("Failed to kill")
			return err
		}
	}
	args := []string{"./kumara-bin"}

	k.command = exec.Command(args[0], args[1:]...)

	stdout, err := k.command.StdoutPipe()
	if err != nil {
		return nil
	}

	stderr, err := k.command.StderrPipe()
	if err != nil {
		return nil
	}

	log.Print("Starting process")
	err = k.command.Start()
	if err != nil {
		return err
	}

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	go k.command.Wait()

	return nil
}

func (k Kumara) Watch() {
	for {
		select {
		case event := <-k.watcher.Events:
			log.Print("event: ", event.Op, " - ", event.Name)
			if compareOp(event.Op, fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err != nil {
					log.Print(err)
					break
				}
				if info.IsDir() {
					k.Add(event.Name)
				}
			}
			if !compareOp(event.Op, fsnotify.Chmod) && strings.HasSuffix(event.Name, ".go") {
				err := k.Build()
				if err != nil {
					log.Print(err)
					break
				}
				err = k.Restart()
				if err != nil {
					log.Print(err)
					break
				}
			}
		case err := <-k.watcher.Errors:
			log.Print("error:", err)
		}
	}
}

func compareOp(a, b fsnotify.Op) bool {
	return a&b == b
}
