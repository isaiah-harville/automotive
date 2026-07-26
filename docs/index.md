# Automotive Flow

A library of [Numaflow](https://numaflow.numaproj.io/) pipeline components
for automotive plant use, centered on ECU flashing stations over CAN/UDS.
Pipelines are authored directly as Numaflow `Pipeline` YAML — there is no
visual builder yet.

There is no real hardware wired up yet. The protocol stack (CAN transport →
ISO-TP framing → UDS client) is built against a swappable transport
interface and validated end-to-end with a simulated ECU, so it is ready to
point at real hardware once a plant has it. See [Readiness
Audit](readiness-audit.md) for a candid list of what is and isn't real
today.

## Where to start

- **New to CAN/UDS or Numaflow?** [Glossary](glossary.md) covers the
  terminology used throughout these docs, from CAN frames up through UDS
  services and Numaflow's pipeline/vertex model.
- **New to the project?** Read [Architecture](architecture.md) for how the
  pieces fit together, then [Component Library](components.md) for the
  catalog of what's built.
- **Building a new component or UDS service?** [Component
  Library](components.md) has the extension recipe.
- **Deploying somewhere?** [Configuration](configuration.md) covers every
  environment variable, and [Local Deployment](local-deployment.md) walks
  through running the example pipeline in a local Minikube cluster via the
  devcontainer.
- **Wondering what's production-ready vs. simulated?** [Readiness
  Audit](readiness-audit.md).

## API reference

This site is curated documentation — architecture, contracts, operations —
not generated API docs. For the source-level Go API reference (every
exported type, function, and method), read the package doc comments
directly:

```sh
go doc ./pkg/uds
go doc -all ./pkg/uds
```

or browse the equivalent in your editor via `gopls`. This module is not
published to a public registry — it's a private, source-available
commercial codebase, not open source.
