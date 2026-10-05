# tinycode v1.20 - LinkedIn Post Draft

**Target:** LinkedIn
**Audience:** Tech community, Red Hat colleagues, customers, conference reviewers
**Tone:** Professional builder, not product launch. Show craft, not hype.

---

I wanted an AI coding assistant that worked like vi - light, fast, no login, no subscription, no internet required. Something I could use air-gapped on a plane or in a disconnected lab. Every option I found either required frontier API tokens or phoned home to a cloud service. So I built one.

tinycode runs local-first against your own models - Ollama, vLLM, ramalama, whatever's on your machine or LAN. No API keys, no data leaving your network, no account. It also connects to cloud providers when you want them, but it doesn't need them.

What started as a fork has turned into a five-repo ecosystem:

- A TUI, web UI (SolidJS), and Electron desktop app - same engine, three surfaces
- 24 specialized agents (code review, security, debugging, cluster admin, architecture)
- 30 plugins, including integrations for OpenShift, Ansible, RHACS, Tekton, and Satellite
- A multi-arch container image (UBI9, amd64 + arm64) that runs on OpenShift with arbitrary UIDs
- A Kubernetes Operator that manages instances via CRDs - handles SCCs, Routes, PVCs, cross-namespace vLLM discovery, and self-service provisioning

v1.20 added GPU-aware auto-profiling for Ollama (detects your VRAM, creates optimized context windows), a command palette with frecency ranking, and OpenRouter support with per-session cost tracking.

Building it also made me a better Solution Architect. You understand containerization, Operators, SCCs, and multi-arch builds differently when you're the one shipping them - not just advising on them.

Truly open source. Homebrew tap, npx install, container image on Quay, or clone and build with Bun.

https://bobbyjohnstx.github.io/tinycode.html

#OpenShift #Kubernetes #AI #LocalLLM #OpenSource
