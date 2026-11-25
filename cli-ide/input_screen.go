package main

import "github.com/gdamore/tcell/v2"

type inputScreen struct {
	tcell.Screen
	initErr       error
	skipFrame     bool
	retainFrame   bool
	width, height int
}

func (s *inputScreen) Init() error {
	s.initErr = s.Screen.Init()
	return s.initErr
}

func (s *inputScreen) Clear() {
	if !s.skipFrame && !s.retainFrame {
		s.Screen.Clear()
	}
}

func (s *inputScreen) Show() {
	s.retainFrame = false
	if s.skipFrame {
		s.skipFrame = false
		return
	}
	s.Screen.Show()
}
