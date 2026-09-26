package app

import "embed"

//go:embed templates/*
var templatesFS embed.FS

func templateString(name string) (string, error) {
	b, err := templatesFS.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
