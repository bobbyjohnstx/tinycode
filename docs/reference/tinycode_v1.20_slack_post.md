# tinycode v1.20 - Internal Slack Post Draft

**Target:** SA community channel or OpenShift/builders channel (pick the one where people share projects)
**Audience:** Red Hat SAs, SSPs, technical peers
**Tone:** Peer-to-peer, casual, show don't tell

---

Shipped v1.20 of tinycode - quick update since I first mentioned it on cloud-strategy.

I wanted an AI coding tool that worked like vi - no login, no subscription, runs air-gapped. Couldn't find one that didn't require frontier API tokens or phone home to a cloud service, so I built one. It runs local-first against Ollama, vLLM, or ramalama on your LAN. No data leaves your network.

What started as a simple fork, has grown into a five-repo ecosystem: core app (TUI + web + Electron), a UBI9 multi-arch container image, a Kubernetes Operator, 30 plugins (25 are Red Hat product integrations - OpenShift, Ansible, RHACS, Tekton, Satellite), and a Homebrew tap.

The Operator manages TinycodeInstance CRDs on OpenShift - SCCs, Routes, PVCs, cross-namespace vLLM auto-discovery, self-service provisioning. Building it made me a lot more empathetic to what our customers go through with Operators and SCCs.

New in v1.20: GPU-aware auto-profiling for Ollama (detects your VRAM, caps context windows to fit), command palette with frecency, OpenRouter support with 300+ models and per-session cost tracking.


More Details: https://bobbyjohnstx.github.io/tinycode.html
