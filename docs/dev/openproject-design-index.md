# OpenProject design index

Status: current.

These documents record the design, reviews and results behind the OpenProject
side of RePlan. Read the current documents first. Dated records explain past
decisions and evidence; superseded documents describe interfaces that the
current implementation replaced.

The live backlog is in OpenProject and `replan next`, not in these documents.

## Agent interface and CLI layer

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject agent interface options](openproject-agent-interface-options.md) | dated record (2026-08-04) | Evaluation that selected RePlan-owned guarded helpers over external clients. | See [AI CLI operating layer](openproject-ai-cli-operating-layer-design.md). |
| [OpenProject agent tools hands-on audit](openproject-agent-tools-hands-on-audit.md) | dated record (2026-08-04) | Hands-on audit of the official CLI, MCP and third-party tools. | See [AI CLI operating layer](openproject-ai-cli-operating-layer-design.md). |
| [OpenProject AI CLI operating layer design](openproject-ai-cli-operating-layer-design.md) | current | Guarded read, preview and apply contract; first-class Go packaging remains future work. | See [work-package toolkit](openproject-work-package-operator-toolkit-design.md) and the [agent kit](../agent-kit/README.md#surface-map). |

## Work-package toolkit and link

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject work-package operator toolkit design](openproject-work-package-operator-toolkit-design.md) | current | Detailed contract implemented by the transitional work-package toolkit script. | See [AI CLI operating layer](openproject-ai-cli-operating-layer-design.md) and the [agent kit](../agent-kit/README.md#surface-map). |
| [Reaching the RePlan task page from a work package](openproject-work-package-link-research.md) | dated record (2026-09-27) | Extension-point research and the still-current link-custom-field choice. | See [plugin redesign](openproject-plugin-redesign-design.md#work-package-panel). |

## Overlay plugin architecture and deployment

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject-native RePlan surface design](openproject-native-replan-surface-design.md) | current | Staged surface strategy; its Level 3 view details are amended by the redesign. | See [plugin feasibility](openproject-read-only-plugin-feasibility.md) and [plugin redesign](openproject-plugin-redesign-design.md). |
| [OpenProject read-only RePlan plugin feasibility](openproject-read-only-plugin-feasibility.md) | current | Read-only renderer boundary and external overlay-artifact decision. | See [deployment path](openproject-plugin-deployment-path.md). |
| [OpenProject RePlan overlay plugin consumer scaffold](openproject-overlay-plugin-consumer-scaffold.md) | superseded by [plugin redesign](openproject-plugin-redesign-design.md) | First minimal project-page renderer and version 1 view contract. | Replaced by the overview, Gantt and task-page design. |
| [OpenProject RePlan Plugin Deployment Path](openproject-plugin-deployment-path.md) | current | Exact-version image, artifact delivery, preflight and rollback design. | See [throwaway runtime test](openproject-plugin-throwaway-runtime-test.md) and [live result](openproject-plugin-live-installation-result.md). |
| [OpenProject RePlan Plugin Live Installation Preflight](openproject-plugin-live-installation-preflight.md) | dated record (2026-08-07) | Preflight evidence before the first live overlay-plugin installation. | See [live result](openproject-plugin-live-installation-result.md). |
| [OpenProject RePlan Plugin Live Installation Result](openproject-plugin-live-installation-result.md) | dated record (2026-08-07) | Rolling deployment record for the first installation and later plugin milestones. | See [deployment path](openproject-plugin-deployment-path.md). |
| [OpenProject RePlan Plugin Throwaway Runtime Test](openproject-plugin-throwaway-runtime-test.md) | dated record (2026-08-07) | Runtime proof and the later rendered-page test history. | See [deployment path](openproject-plugin-deployment-path.md). |

## Overlay views and work-package panel

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject work-package RePlan panel design](openproject-work-package-replan-panel-design.md) | superseded by [plugin redesign](openproject-plugin-redesign-design.md) | First count-and-strip work-package panel design. | Replaced by the task page with a dated axis, evidence, actuals and dependencies. |
| [OpenProject RePlan plugin review 2026-09-25](openproject-plugin-review-2026-09-25.md) | dated record (2026-09-25) | Operator-backed review that found the first plugin views inadequate. | Input to [plugin redesign](openproject-plugin-redesign-design.md). |
| [OpenProject RePlan plugin UI audit 2026-09-25](openproject-plugin-ui-audit-2026-09-25.md) | dated record (2026-09-25) | Measurements and visual findings for the old project page and panel. | Input to [plugin redesign](openproject-plugin-redesign-design.md). |
| [OpenProject RePlan plugin redesign](openproject-plugin-redesign-design.md) | current | Current overview, Gantt, load-profile, task-page and overlay v2 design, with accepted amendments. | Verified by the [2026-10-15 UI audit](openproject-plugin-ui-audit-2026-10-15.md). |
| [OpenProject RePlan plugin UI audit 2026-10-15](openproject-plugin-ui-audit-2026-10-15.md) | dated record (2026-10-15) | Acceptance audit of the finished redesigned plugin. | See [plugin redesign](openproject-plugin-redesign-design.md). |

## Wiki API

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject wiki API gap](openproject-wiki-api-gap.md) | dated record (2026-09-21) | Original platform-gap and acceptance-criterion record; the gap was later closed. | See [live deployment result](openproject-wiki-api-live-deployment-result.md#acceptance). |
| [OpenProject wiki API plugin design review](openproject-wiki-api-plugin-design-review.md) | dated record (2026-09-21) | Review that required amendments before implementation. | Amendments are incorporated in the [plugin design](openproject-wiki-api-plugin-design.md#review-amendments-applied). |
| [OpenProject wiki API plugin design](openproject-wiki-api-plugin-design.md) | current | Core API, permission, locking and implementation contract; delete and guarded client additions came later. | See [throwaway proof](openproject-wiki-api-throwaway-runtime-test.md#update-2026-09-25-delete-route-cases) and the [agent kit wiki workflow](../agent-kit/README.md#the-project-wiki). |
| [OpenProject wiki API plugin implementation review](openproject-wiki-api-plugin-implementation-review.md) | dated record (2026-09-21) | Gate 1 review, fixes and re-review. | See [throwaway proof](openproject-wiki-api-throwaway-runtime-test.md). |
| [OpenProject wiki API plugin throwaway runtime test](openproject-wiki-api-throwaway-runtime-test.md) | dated record (2026-09-21) | Disposable-runtime HTTP, permission, locking and delete evidence. | See [live deployment result](openproject-wiki-api-live-deployment-result.md). |
| [OpenProject wiki API plugin live deployment result](openproject-wiki-api-live-deployment-result.md) | dated record (2026-09-21) | Sanitized live deployment and acceptance record, with later rollout updates. | See [plugin design](openproject-wiki-api-plugin-design.md). |

## OpenProject behaviour and pitfalls

| Document | Status | What it is | Superseded by / see also |
| --- | --- | --- | --- |
| [OpenProject pilot lessons](openproject-pilot-lessons.md) | current | Sanitized field, endpoint, sidecar, accept-flow and mapping-quality lessons retained from private validation. | See the [operator loop](../spec/openproject-operator-loop.md), [CLI contract](../spec/cli.md) and [agent kit](../agent-kit/README.md). |
| [OpenProject functionality and technical pitfalls](openproject-functionality-and-pitfalls.md) | current | Current ownership, locking, relation, date and binding reference; its preflight scope and MCP snapshot are historical. | See the [agent kit direction rule](../agent-kit/README.md#date-and-status-direction). |
