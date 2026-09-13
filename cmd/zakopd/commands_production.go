//go:build !networkdebug

package main

const buildUsage = ""

func runBuildCommand(args []string) (bool, error) { return false, nil }
