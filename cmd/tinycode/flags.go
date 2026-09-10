package main

import "flag"

type commonFlags struct {
	model string
}

func parseCommonFlags(name string, args []string) commonFlags {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	modelFlag := fs.String("m", "", "model to use (provider/model)")
	fs.StringVar(modelFlag, "model", "", "model to use (provider/model)")
	_ = fs.Parse(args)
	return commonFlags{model: *modelFlag}
}
