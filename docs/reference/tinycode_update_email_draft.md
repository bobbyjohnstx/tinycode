# tinycode Update Email - Draft

**Status:** DRAFT - awaiting Bobby's review
**Channel:** Reply-all to existing thread on cloud-strategy@redhat.com + ai-strategy@redhat.com
**Subject:** Re: tinycode - a local-first AI coding agent that runs /w zero frontier tokens

---

Team -

Quick update on tinycode since the July thread. A few of you asked for specific things - here's what landed.

**Red Hat Plugins.** 25 plugins across 8 product categories - OpenShift cluster ops, RHACS security scanning, AAP job management, Tekton pipelines, RHOAI model serving, RHACM fleet management, Satellite, and Quay. They're in a separate repo (tinycode-plugins) and install with `tinycode plugin add <name>`. The idea: your coding session is already connected to a terminal, so why not connect it to the cluster too. Authenticate once with `ocp-oauth`, and every plugin reuses the token.

The one I'm most interested in testing: `eda-events` bridges tinycode session events directly to Event-Driven Ansible. Build a container image, push code, edit a manifest - EDA picks it up and runs your rulebook. No webhook plumbing.

**OpenShell integration.** NVIDIA open-sourced OpenShell (Apache 2.0) - a kernel-level sandbox for AI coding agents using Landlock and seccomp. tinycode now runs inside it. The practical effect: the agent gets filesystem and network isolation enforced at the kernel, not just by "are you sure?" prompts. For anyone running this in a shared environment or on a customer cluster, that's the missing piece.

**Plugin SDK.** Published to npm as `tinycode-plugin`. Scaffold a new plugin with `tinycode plugin-init`, write your tools, test with the mock harness, ship it. Jerome and Simon - you both mentioned productization and CNI/air-gapped use cases. The plugin system is how that scales without me being a bottleneck.

**Other stuff since July:** OpenRouter support (300+ models), LM Studio auto-discovery, unified command palette, Electron desktop app, subagent depth limiting, and a bunch of stability fixes. Full changelog is in the repo.

Edward - you asked about a research skill for local docs. That's partly there now through the `rh-dev-content` plugin (searches Red Hat developer articles, cheatsheets, learning paths locally). The broader "point it at any doc directory" version is on the backlog.

Christoph - still no demo video, but the install is `npx tinycode` or `brew install tinycode` now. Five minutes to running if you have Ollama.

I haven't tested the plugins against a live cluster yet - that's this week. If anyone wants to beat me to it, the plugin README has suggested bundles by role (OCP admin, platform/SRE, app dev, security, AI/ML engineer, etc.). Issues and PRs welcome.

Repos:
- Core: github.com/bobbyjohnstx/tinycode
- Plugins: github.com/bobbyjohnstx/tinycode-plugins
- Container: github.com/bobbyjohnstx/tinycode-container
- Operator: github.com/bobbyjohnstx/tinycode-operator

-- Bobby
