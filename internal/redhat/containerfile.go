package redhat

import (
	"regexp"
	"strconv"
	"strings"
)

type InstructionType string

const (
	InstrFrom       InstructionType = "FROM"
	InstrRun        InstructionType = "RUN"
	InstrCopy       InstructionType = "COPY"
	InstrAdd        InstructionType = "ADD"
	InstrEnv        InstructionType = "ENV"
	InstrLabel      InstructionType = "LABEL"
	InstrUser       InstructionType = "USER"
	InstrExpose     InstructionType = "EXPOSE"
	InstrArg        InstructionType = "ARG"
	InstrWorkdir    InstructionType = "WORKDIR"
	InstrEntrypoint InstructionType = "ENTRYPOINT"
	InstrCmd        InstructionType = "CMD"
	InstrHealthcheck InstructionType = "HEALTHCHECK"
	InstrVolume     InstructionType = "VOLUME"
	InstrStopsignal InstructionType = "STOPSIGNAL"
	InstrShell      InstructionType = "SHELL"
	InstrOnbuild    InstructionType = "ONBUILD"
)

type Instruction struct {
	Type       InstructionType
	LineNumber int

	// FROM fields
	Image  string
	Tag    string
	Digest string
	Alias  string

	// RUN, WORKDIR, ENTRYPOINT, CMD, etc.
	Command string
	Value   string

	// COPY/ADD fields
	Src      string
	Dest     string
	CopyFrom string

	// ENV/LABEL fields
	Key      string
	EnvValue string

	// USER field
	User string

	// EXPOSE fields
	Port     int
	Protocol string

	// ARG fields
	ArgName      string
	DefaultValue string
}

type Stage struct {
	From         Instruction
	Instructions []Instruction
}

type ParsedContainerfile struct {
	Stages     []Stage
	GlobalArgs []Instruction
}

type DependencySource string

const (
	DepPip   DependencySource = "pip"
	DepNpm   DependencySource = "npm"
	DepDnf   DependencySource = "dnf"
	DepMaven DependencySource = "maven"
	DepGo    DependencySource = "go"
)

type Dependency struct {
	Name    string
	Version string
	Source  DependencySource
}

var otherDirectives = map[string]bool{
	"WORKDIR": true, "ENTRYPOINT": true, "CMD": true, "HEALTHCHECK": true,
	"VOLUME": true, "STOPSIGNAL": true, "SHELL": true, "ONBUILD": true,
}

type mergedLine struct {
	text       string
	lineNumber int
}

func mergeLines(rawLines []string) []mergedLine {
	var merged []mergedLine
	var current string
	startLine := 0
	inHeredoc := false
	heredocMarker := ""
	heredocRe := regexp.MustCompile(`<<-?\s*['"]?(\w+)['"]?`)

	for i, line := range rawLines {
		if inHeredoc {
			current += "\n" + line
			if strings.TrimSpace(line) == heredocMarker {
				inHeredoc = false
				merged = append(merged, mergedLine{text: current, lineNumber: startLine})
				current = ""
			}
			continue
		}

		if current == "" {
			startLine = i + 1
			current = line
		} else {
			current += " " + strings.TrimSpace(line)
		}

		if m := heredocRe.FindStringSubmatch(current); m != nil {
			inHeredoc = true
			heredocMarker = m[1]
			continue
		}

		if strings.HasSuffix(current, "\\") {
			current = strings.TrimRight(current[:len(current)-1], " ")
			continue
		}

		merged = append(merged, mergedLine{text: current, lineNumber: startLine})
		current = ""
	}

	if current != "" {
		merged = append(merged, mergedLine{text: current, lineNumber: startLine})
	}

	return merged
}

func parseFrom(args string, lineNumber int) Instruction {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return Instruction{Type: InstrFrom, LineNumber: lineNumber}
	}
	imageRef := parts[0]
	instr := Instruction{Type: InstrFrom, LineNumber: lineNumber}

	if len(parts) >= 3 && strings.EqualFold(parts[1], "AS") {
		instr.Alias = parts[2]
	}

	if idx := strings.Index(imageRef, "@"); idx != -1 {
		instr.Image = imageRef[:idx]
		instr.Digest = imageRef[idx+1:]
		return instr
	}

	if idx := strings.Index(imageRef, ":"); idx != -1 {
		instr.Image = imageRef[:idx]
		instr.Tag = imageRef[idx+1:]
		return instr
	}

	instr.Image = imageRef
	return instr
}

func parseCopyAdd(directive InstructionType, args string, lineNumber int) Instruction {
	instr := Instruction{Type: directive, LineNumber: lineNumber}
	rest := strings.TrimSpace(args)

	fromRe := regexp.MustCompile(`^--from=(\S+)\s+`)
	if m := fromRe.FindStringSubmatch(rest); m != nil {
		instr.CopyFrom = m[1]
		rest = rest[len(m[0]):]
	}

	parts := strings.Fields(rest)
	if len(parts) > 0 {
		instr.Dest = parts[len(parts)-1]
		instr.Src = strings.Join(parts[:len(parts)-1], " ")
	}

	return instr
}

var kvRe = regexp.MustCompile(`(\S+?)=(?:["']([^"']*)["']|(\S*))`)

func parseKeyValuePairs(args string) []struct{ Key, Value string } {
	matches := kvRe.FindAllStringSubmatch(args, -1)
	var pairs []struct{ Key, Value string }
	for _, m := range matches {
		key := m[1]
		value := m[2]
		if value == "" {
			value = m[3]
		}
		pairs = append(pairs, struct{ Key, Value string }{key, value})
	}
	return pairs
}

func parseEnv(args string, lineNumber int) []Instruction {
	pairs := parseKeyValuePairs(args)
	if len(pairs) > 0 {
		var instrs []Instruction
		for _, p := range pairs {
			instrs = append(instrs, Instruction{
				Type: InstrEnv, Key: p.Key, EnvValue: p.Value, LineNumber: lineNumber,
			})
		}
		return instrs
	}

	idx := strings.Index(args, " ")
	if idx != -1 {
		return []Instruction{{
			Type: InstrEnv, Key: strings.TrimSpace(args[:idx]),
			EnvValue: strings.TrimSpace(args[idx+1:]), LineNumber: lineNumber,
		}}
	}

	return []Instruction{{Type: InstrEnv, Key: strings.TrimSpace(args), LineNumber: lineNumber}}
}

func parseLabel(args string, lineNumber int) []Instruction {
	pairs := parseKeyValuePairs(args)
	if len(pairs) > 0 {
		var instrs []Instruction
		for _, p := range pairs {
			instrs = append(instrs, Instruction{
				Type: InstrLabel, Key: p.Key, EnvValue: p.Value, LineNumber: lineNumber,
			})
		}
		return instrs
	}

	return []Instruction{{Type: InstrLabel, Key: strings.TrimSpace(args), LineNumber: lineNumber}}
}

func parseExpose(args string, lineNumber int) Instruction {
	parts := strings.SplitN(strings.TrimSpace(args), "/", 2)
	port, _ := strconv.Atoi(parts[0])
	instr := Instruction{Type: InstrExpose, Port: port, LineNumber: lineNumber}
	if len(parts) > 1 {
		instr.Protocol = parts[1]
	}
	return instr
}

func parseArg(args string, lineNumber int) Instruction {
	idx := strings.Index(args, "=")
	if idx != -1 {
		val := strings.TrimSpace(args[idx+1:])
		val = strings.Trim(val, "\"'")
		return Instruction{
			Type: InstrArg, ArgName: strings.TrimSpace(args[:idx]),
			DefaultValue: val, LineNumber: lineNumber,
		}
	}
	return Instruction{Type: InstrArg, ArgName: strings.TrimSpace(args), LineNumber: lineNumber}
}

func parseInstruction(text string, lineNumber int) []Instruction {
	instrRe := regexp.MustCompile(`^(\S+)\s*(.*)$`)
	m := instrRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}

	directive := strings.ToUpper(m[1])
	args := m[2]

	switch directive {
	case "FROM":
		return []Instruction{parseFrom(args, lineNumber)}
	case "RUN":
		return []Instruction{{Type: InstrRun, Command: strings.TrimSpace(args), LineNumber: lineNumber}}
	case "COPY":
		return []Instruction{parseCopyAdd(InstrCopy, args, lineNumber)}
	case "ADD":
		return []Instruction{parseCopyAdd(InstrAdd, args, lineNumber)}
	case "ENV":
		return parseEnv(args, lineNumber)
	case "LABEL":
		return parseLabel(args, lineNumber)
	case "USER":
		return []Instruction{{Type: InstrUser, User: strings.TrimSpace(args), LineNumber: lineNumber}}
	case "EXPOSE":
		return []Instruction{parseExpose(args, lineNumber)}
	case "ARG":
		return []Instruction{parseArg(args, lineNumber)}
	default:
		if otherDirectives[directive] {
			return []Instruction{{
				Type: InstructionType(directive), Value: strings.TrimSpace(args), LineNumber: lineNumber,
			}}
		}
		return nil
	}
}

func ParseContainerfile(content string) ParsedContainerfile {
	rawLines := strings.Split(content, "\n")
	merged := mergeLines(rawLines)
	var stages []Stage
	var globalArgs []Instruction
	var currentStage *Stage

	for _, ml := range merged {
		trimmed := strings.TrimSpace(ml.text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		instructions := parseInstruction(trimmed, ml.lineNumber)
		for _, instr := range instructions {
			if instr.Type == InstrFrom {
				stages = append(stages, Stage{From: instr, Instructions: []Instruction{instr}})
				currentStage = &stages[len(stages)-1]
			} else if currentStage == nil {
				if instr.Type == InstrArg {
					globalArgs = append(globalArgs, instr)
				}
			} else {
				currentStage.Instructions = append(currentStage.Instructions, instr)
			}
		}
	}

	return ParsedContainerfile{Stages: stages, GlobalArgs: globalArgs}
}

func ExtractDependencies(parsed ParsedContainerfile) []Dependency {
	var deps []Dependency
	for _, stage := range parsed.Stages {
		for _, instr := range stage.Instructions {
			if instr.Type != InstrRun {
				continue
			}
			extractFromCommand(instr.Command, &deps)
		}
	}
	return deps
}

func extractFromCommand(command string, deps *[]Dependency) {
	extractPipDeps(command, deps)
	extractNpmDeps(command, deps)
	extractDnfDeps(command, deps)
	extractMavenDeps(command, deps)
	extractGoDeps(command, deps)
}

var pipRe = regexp.MustCompile(`pip3?\s+install\s+(.+?)(?:\s*&&|$)`)

func extractPipDeps(command string, deps *[]Dependency) {
	m := pipRe.FindStringSubmatch(command)
	if m == nil {
		return
	}
	for _, token := range strings.Fields(m[1]) {
		if strings.HasPrefix(token, "-") || strings.Contains(token, "requirements") || strings.HasSuffix(token, ".txt") {
			continue
		}
		if parts := strings.SplitN(token, "==", 2); len(parts) == 2 {
			*deps = append(*deps, Dependency{Name: parts[0], Version: parts[1], Source: DepPip})
		} else if parts := strings.SplitN(token, ">=", 2); len(parts) == 2 {
			*deps = append(*deps, Dependency{Name: parts[0], Version: ">=" + parts[1], Source: DepPip})
		} else {
			*deps = append(*deps, Dependency{Name: token, Source: DepPip})
		}
	}
}

var npmRe = regexp.MustCompile(`npm\s+install\s+(.+?)(?:\s*&&|$)`)

func extractNpmDeps(command string, deps *[]Dependency) {
	m := npmRe.FindStringSubmatch(command)
	if m == nil {
		return
	}
	for _, token := range strings.Fields(m[1]) {
		if strings.HasPrefix(token, "-") {
			continue
		}
		if idx := strings.LastIndex(token, "@"); idx > 0 {
			*deps = append(*deps, Dependency{Name: token[:idx], Version: token[idx+1:], Source: DepNpm})
		} else {
			*deps = append(*deps, Dependency{Name: token, Source: DepNpm})
		}
	}
}

var dnfRe = regexp.MustCompile(`(?:dnf|yum)\s+install\s+(?:-y\s+)?(.+?)(?:\s*&&|$)`)

func extractDnfDeps(command string, deps *[]Dependency) {
	m := dnfRe.FindStringSubmatch(command)
	if m == nil {
		return
	}
	for _, token := range strings.Fields(m[1]) {
		if strings.HasPrefix(token, "-") {
			continue
		}
		*deps = append(*deps, Dependency{Name: token, Source: DepDnf})
	}
}

var mavenRe = regexp.MustCompile(`mvn\s+.*dependency:resolve`)

func extractMavenDeps(command string, deps *[]Dependency) {
	if mavenRe.MatchString(command) {
		*deps = append(*deps, Dependency{Name: "maven-dependencies", Source: DepMaven})
	}
}

var goDepRe = regexp.MustCompile(`go\s+(?:get|mod\s+download)\s+(.+?)(?:\s*&&|$)`)

func extractGoDeps(command string, deps *[]Dependency) {
	m := goDepRe.FindStringSubmatch(command)
	if m == nil {
		return
	}
	for _, token := range strings.Fields(m[1]) {
		if strings.HasPrefix(token, "-") {
			continue
		}
		if idx := strings.LastIndex(token, "@"); idx > 0 {
			*deps = append(*deps, Dependency{Name: token[:idx], Version: token[idx+1:], Source: DepGo})
		} else {
			*deps = append(*deps, Dependency{Name: token, Source: DepGo})
		}
	}
}
