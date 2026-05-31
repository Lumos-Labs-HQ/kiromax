package ui

import "fmt"

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	cyan   = "\033[36m"
)

func Bold(s string) string   { return bold + s + reset }
func Dim(s string) string    { return dim + s + reset }
func Green(s string) string  { return green + s + reset }
func Yellow(s string) string { return yellow + s + reset }
func Red(s string) string    { return red + s + reset }
func Cyan(s string) string   { return cyan + s + reset }

func Success(msg string) { fmt.Println(green+bold+"✓"+reset + " " + msg) }
func Info(msg string)    { fmt.Println(cyan+bold+"→"+reset + " " + msg) }
func Fail(msg string)    { fmt.Println(red+bold+"✗"+reset + " " + msg) }
