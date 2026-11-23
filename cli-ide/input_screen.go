package main

import "github.com/gdamore/tcell/v2"

type inputScreen struct {
	tcell.Screen
	initErr   error
	skipFrame bool
}

func (s *inputScreen) Init() error {
	s.initErr = s.Screen.Init()
	return s.initErr
}

func (s *inputScreen) Clear() {
	if !s.skipFrame {
		s.Screen.Clear()
	}
}

func (s *inputScreen) Show() {
	if s.skipFrame {
		s.skipFrame = false
		return
	}
	s.Screen.Show()
}
