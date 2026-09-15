package main

import "strings"

// HAProxyConfig represents a parsed HAProxy configuration file.
type HAProxyConfig struct {
	Global    HAProxySection
	Defaults  HAProxySection
	Frontends []HAProxyFrontend
	Backends  []HAProxyBackend
}

// HAProxySection holds global or defaults directives.
type HAProxySection struct {
	Timeouts map[string]string
	MaxConn  string
	Mode     string
	Options  []string
}

// HAProxyFrontend represents a frontend section.
type HAProxyFrontend struct {
	Name  string
	Mode  string
	Binds []string
}

// HAProxyBackend represents a backend section.
type HAProxyBackend struct {
	Name     string
	Mode     string
	Balance  string
	Servers  []HAProxyServer
	Timeouts map[string]string
}

// HAProxyServer represents a server directive within a backend.
type HAProxyServer struct {
	Name    string
	Address string
	Options string
}

// parseHAProxyConfig parses an HAProxy configuration string into structured data.
func parseHAProxyConfig(data string) *HAProxyConfig {
	cfg := &HAProxyConfig{
		Global:   HAProxySection{Timeouts: make(map[string]string)},
		Defaults: HAProxySection{Timeouts: make(map[string]string)},
	}

	type sectionKind int
	const (
		sNone sectionKind = iota
		sGlobal
		sDefaults
		sFrontend
		sBackend
	)

	var section sectionKind
	var curFE *HAProxyFrontend
	var curBE *HAProxyBackend

	flush := func() {
		if curBE != nil {
			cfg.Backends = append(cfg.Backends, *curBE)
			curBE = nil
		}
		if curFE != nil {
			cfg.Frontends = append(cfg.Frontends, *curFE)
			curFE = nil
		}
	}

	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		switch {
		case trimmed == "global":
			flush()
			section = sGlobal
			continue
		case trimmed == "defaults":
			flush()
			section = sDefaults
			continue
		case strings.HasPrefix(trimmed, "frontend "):
			flush()
			curFE = &HAProxyFrontend{Name: strings.TrimPrefix(trimmed, "frontend ")}
			section = sFrontend
			continue
		case strings.HasPrefix(trimmed, "backend "):
			flush()
			curBE = &HAProxyBackend{
				Name:     strings.TrimPrefix(trimmed, "backend "),
				Timeouts: make(map[string]string),
			}
			section = sBackend
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}

		switch section {
		case sGlobal:
			parseSectionDirective(&cfg.Global, fields, trimmed)
		case sDefaults:
			parseSectionDirective(&cfg.Defaults, fields, trimmed)
		case sFrontend:
			if curFE != nil {
				switch fields[0] {
				case "bind":
					curFE.Binds = append(curFE.Binds, strings.TrimPrefix(trimmed, "bind "))
				case "mode":
					if len(fields) >= 2 {
						curFE.Mode = fields[1]
					}
				}
			}
		case sBackend:
			if curBE != nil {
				switch fields[0] {
				case "mode":
					if len(fields) >= 2 {
						curBE.Mode = fields[1]
					}
				case "balance":
					if len(fields) >= 2 {
						curBE.Balance = fields[1]
					}
				case "server":
					srv := HAProxyServer{}
					if len(fields) >= 2 {
						srv.Name = fields[1]
					}
					if len(fields) >= 3 {
						srv.Address = fields[2]
					}
					if len(fields) > 3 {
						srv.Options = strings.Join(fields[3:], " ")
					}
					curBE.Servers = append(curBE.Servers, srv)
				case "timeout":
					if len(fields) >= 3 {
						curBE.Timeouts[fields[1]] = fields[2]
					}
				}
			}
		}
	}

	flush()
	return cfg
}

func parseSectionDirective(s *HAProxySection, fields []string, line string) {
	if len(fields) < 2 {
		s.Options = append(s.Options, line)
		return
	}
	switch fields[0] {
	case "timeout":
		if len(fields) >= 3 {
			s.Timeouts[fields[1]] = fields[2]
		}
	case "maxconn":
		s.MaxConn = fields[1]
	case "mode":
		s.Mode = fields[1]
	default:
		s.Options = append(s.Options, line)
	}
}
