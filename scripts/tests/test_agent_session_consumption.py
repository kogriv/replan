"""Synthetic-only tests for agent-session-consumption.sh."""

import json
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
HELPER = ROOT / "scripts" / "agent-session-consumption.sh"


class AgentSessionConsumptionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="agent-session-consumption-")
        self.work = Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def write_jsonl(self, relative, rows, raw_lines=()):
        path = self.work / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("w", encoding="utf-8") as stream:
            for row in rows:
                stream.write(json.dumps(row) + "\n")
            for line in raw_lines:
                stream.write(line + "\n")
        return path

    def run_helper(self, *args, env=None):
        run_env = os.environ.copy()
        run_env.pop("REPLAN_BIN", None)
        run_env["TMPDIR"] = str(self.work)
        if env:
            run_env.update(env)
        proc = subprocess.run(
            [str(HELPER), *map(str, args)], cwd=ROOT, env=run_env,
            text=True, capture_output=True, check=False,
        )
        self.assertFalse(proc.stderr, proc.stderr)
        try:
            payload = json.loads(proc.stdout)
        except ValueError:
            self.fail(f"not JSON: rc={proc.returncode} stdout={proc.stdout!r}")
        return proc.returncode, payload

    @staticmethod
    def codex_meta(timestamp, session, cwd="/workspace/project"):
        return {"timestamp": timestamp, "type": "session_meta", "payload": {"id": session, "cwd": cwd, "originator": "codex_cli"}}

    def test_gap_exactly_threshold_merges_and_just_over_splits(self):
        rollout = self.write_jsonl("codex/rollout.jsonl", [
            self.codex_meta("2026-11-15T10:00:00Z", "one"),
            {"timestamp": "2026-11-15T10:05:00Z", "type": "event_msg"},
            {"timestamp": "2026-11-15T10:10:00.001Z", "type": "response_item"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T09:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--codex", rollout,
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][1]
        self.assertEqual(resource["seconds"], 300)
        self.assertEqual([item["seconds"] for item in resource["intervals"]], [300, 0])

    def test_window_clipping_malformed_untimestamped_and_cwd_filter(self):
        rollout = self.write_jsonl("codex/rollout.jsonl", [
            self.codex_meta("2026-11-15T09:58:00Z", "kept", "/keep/project"),
            {"timestamp": "2026-11-15T10:02:00Z", "type": "event_msg"},
            {"type": "token_usage_record"},
        ], raw_lines=("{broken",))
        filtered = self.write_jsonl("codex/filtered.jsonl", [
            self.codex_meta("2026-11-15T10:00:00Z", "filtered", "/elsewhere"),
            {"timestamp": "2026-11-15T10:01:00Z", "type": "event_msg"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T10:03:00Z",
            "--codex", rollout, "--codex", filtered, "--cwd", "/keep",
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][1]
        self.assertEqual(resource["seconds"], 120)
        self.assertEqual(resource["counts"]["outside_window"], 1)
        self.assertEqual(resource["counts"]["no_timestamp"], 1)
        self.assertEqual(resource["counts"]["malformed_json"], 1)
        self.assertEqual(resource["counts"]["cwd_filtered"], 2)
        self.assertIn("malformed_json", out["warning_codes"])

    def test_two_codex_sessions_remain_separate(self):
        directory = self.work / "sessions"
        self.write_jsonl("sessions/day1/one.jsonl", [
            self.codex_meta("2026-11-15T10:00:00Z", "one"),
            {"timestamp": "2026-11-15T10:01:00Z", "type": "event_msg"},
        ])
        self.write_jsonl("sessions/day2/two.jsonl", [
            self.codex_meta("2026-11-15T10:00:30Z", "two"),
            {"timestamp": "2026-11-15T10:02:30Z", "type": "event_msg"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T09:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--codex", directory,
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][1]
        self.assertEqual(resource["seconds"], 150)
        self.assertEqual(resource["session_sum_seconds"], 180)
        self.assertEqual(resource["parallel_seconds"], 30)
        self.assertEqual({item["seconds"] for item in resource["session_totals"]}, {60, 120})
        self.assertEqual(resource["counts"]["sessions"], 2)
        self.assertEqual({item["session_id"] for item in resource["intervals"]}, {"one", "two"})

    def test_claude_missing_session_id_inherits_within_file(self):
        transcript = self.write_jsonl("claude-inherit.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "claude-one", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:01:00Z", "type": "assistant", "cwd": "/p"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T09:00:00Z", "--to", "2026-11-15T11:00:00Z", "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][0]
        self.assertEqual(resource["seconds"], 60)
        self.assertEqual(resource["counts"]["sessions"], 1)
        self.assertEqual(resource["source_files"], [{"file": str(transcript.resolve()), "session_ids": ["claude-one"]}])
        self.assertEqual({item["session_id"] for item in resource["intervals"]}, {"claude-one"})

    def test_parallel_claude_transcripts_use_wall_time_union(self):
        first = self.write_jsonl("claude-parallel/one.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "one", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:02:00Z", "type": "assistant", "sessionId": "one", "cwd": "/p"},
        ])
        second = self.write_jsonl("claude-parallel/two.jsonl", [
            {"timestamp": "2026-11-15T10:01:00Z", "type": "user", "sessionId": "two", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:04:00Z", "type": "assistant", "sessionId": "two", "cwd": "/p"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T09:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--claude", first, "--claude", second,
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][0]
        self.assertEqual(resource["seconds"], 240)
        self.assertEqual(resource["session_sum_seconds"], 300)
        self.assertEqual(resource["parallel_seconds"], 60)
        self.assertEqual(len(resource["session_totals"]), 2)

    def test_claude_other_types_are_not_activity_or_missing_timestamps(self):
        transcript = self.write_jsonl("claude-types.jsonl", [
            {"type": "attachment", "cwd": "/p"},
            {"timestamp": "2026-11-15T10:00:30Z", "type": "system", "cwd": "/p"},
            {"type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "missing timestamp"}},
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:01:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T09:00:00Z", "--to", "2026-11-15T11:00:00Z", "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        resource = out["resources"][0]
        self.assertEqual(resource["seconds"], 60)
        self.assertEqual(resource["counts"]["other_type"], 2)
        self.assertEqual(resource["counts"]["no_timestamp"], 1)

    def test_claude_generation_tool_wait_human_and_overlap(self):
        def row(timestamp, kind, content=None):
            item = {"timestamp": timestamp, "type": kind, "sessionId": "claude-one", "cwd": "/workspace/project"}
            if content is not None:
                item["message"] = {"content": content}
            return item

        transcript = self.write_jsonl("claude.jsonl", [
            row("2026-11-15T10:00:00Z", "user", "start"),
            row("2026-11-15T10:20:00Z", "assistant", [{"type": "tool_use", "id": "tool-one"}]),
            row("2026-11-15T10:30:00Z", "user", [{"type": "tool_result", "tool_use_id": "tool-one", "content": "done"}]),
            row("2026-11-15T10:40:00Z", "user", "next"),
        ])
        codex = self.write_jsonl("codex.jsonl", [
            self.codex_meta("2026-11-15T10:10:00Z", "codex-one"),
            {"timestamp": "2026-11-15T10:15:00Z", "type": "event_msg"},
            {"timestamp": "2026-11-15T10:20:00Z", "type": "response_item"},
            {"timestamp": "2026-11-15T10:25:00Z", "type": "event_msg"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--claude", transcript, "--codex", codex,
        )
        self.assertEqual(rc, 0)
        claude = out["resources"][0]
        self.assertEqual(claude["seconds"], 900)
        self.assertEqual(claude["generation_seconds"], 900)
        self.assertEqual(claude["tool_wait_seconds"], 600)
        self.assertEqual(out["overlap_seconds"], 600)
        self.assertTrue(any(item["human_messages"] == 1 for item in claude["intervals"]))

        rc, counted = self.run_helper(
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--claude", transcript, "--count-tool-waits",
        )
        self.assertEqual(rc, 0)
        self.assertEqual(counted["resources"][0]["seconds"], 1500)

    def test_claude_2x_classification_generation_waits_cwd_and_turn_duration(self):
        session = "claude-2x"

        def row(timestamp, kind, *, content=None, **extra):
            item = {
                "timestamp": timestamp, "type": kind, "sessionId": session,
                "cwd": "/workspace/project", **extra,
            }
            if content is not None:
                item["message"] = {"content": content}
            return item

        transcript = self.write_jsonl("claude-2x.jsonl", [
            row("2026-11-15T10:00:00Z", "user", content="start"),
            {"timestamp": "2026-11-15T10:00:10Z", "type": "api-request", "sessionId": session, "id": "request-one"},
            row("2026-11-15T10:10:10Z", "assistant", content=[{"type": "thinking"}]),
            row("2026-11-15T10:10:20Z", "assistant", content=[{"type": "text"}]),
            row("2026-11-15T10:10:30Z", "assistant", content=[{"type": "tool_use", "id": "tool-one"}]),
            row("2026-11-15T10:20:30Z", "user", content=[{"type": "tool_result", "tool_use_id": "tool-one"}]),
            {"timestamp": "2026-11-15T10:20:40Z", "type": "api-request", "sessionId": session, "id": "request-two"},
            row("2026-11-15T10:21:00Z", "assistant", content=[{"type": "text"}]),
            row("2026-11-15T10:21:10Z", "assistant", content=[{"type": "tool_use", "id": "background-one"}]),
            row("2026-11-15T10:31:10Z", "user", content="<task-notification>finished</task-notification>"),
            row("2026-11-15T10:31:20Z", "user", content="metadata", isMeta=True),
            row("2026-11-15T10:31:30Z", "user", content="<system-reminder>remember</system-reminder>"),
            row("2026-11-15T10:31:31Z", "user", content="<local-command-stdout>output</local-command-stdout>"),
            row("2026-11-15T10:31:32Z", "user", content="<command-name>name</command-name>"),
            row("2026-11-15T10:31:33Z", "user", content="<artifact-content-example>artifact</artifact-content-example>"),
            row("2026-11-15T10:31:40Z", "user", content=[{"type": "text", "text": "real follow-up"}]),
            row("2026-11-15T10:31:50Z", "system", subtype="turn_duration", durationMs=100000, messageCount=82),
            {"timestamp": "2026-11-15T10:05:00Z", "type": "api-request", "sessionId": "orphan", "id": "dropped"},
            {"timestamp": "2026-11-15T10:05:01Z", "type": "api-request-blob", "sessionId": session},
            {"timestamp": "2026-11-15T10:05:02Z", "type": "api-request-shape", "sessionId": session},
        ])
        args = (
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--claude", transcript, "--cwd", "/workspace/project",
        )
        rc, out = self.run_helper(*args)
        self.assertEqual(rc, 0)
        claude = out["resources"][0]
        self.assertEqual(claude["seconds"], 710)
        self.assertEqual(claude["generation_seconds"], 620)
        self.assertEqual(claude["tool_wait_seconds"], 1200)
        self.assertEqual(claude["turn_duration_seconds"], 100)
        self.assertEqual(
            {kind: claude["counts"][kind] for kind in (
                "human", "machine_user", "task_notification", "tool_result",
                "assistant", "api_request", "turn_end",
            )},
            {
                "human": 2, "machine_user": 6, "task_notification": 1,
                "tool_result": 1, "assistant": 5, "api_request": 2,
                "turn_end": 1,
            },
        )
        self.assertEqual(claude["counts"]["cwd_filtered"], 1)
        self.assertEqual(claude["counts"]["other_type"], 2)
        self.assertEqual(claude["counts"]["sessions"], 1)
        self.assertEqual(sum(item["human_messages"] for item in claude["intervals"]), 2)
        self.assertIn("turn_duration_mismatch", out["warning_codes"])

        rc, counted = self.run_helper(*args, "--count-tool-waits")
        self.assertEqual(rc, 0)
        self.assertEqual(counted["resources"][0]["seconds"], 1910)

    def test_old_claude_format_hours_are_unchanged(self):
        transcript = self.write_jsonl("claude-old-format.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "old", "cwd": "/p", "message": {"content": "start"}},
            {"timestamp": "2026-11-15T10:04:00Z", "type": "assistant", "sessionId": "old", "cwd": "/p"},
            {"timestamp": "2026-11-15T10:20:00Z", "type": "user", "sessionId": "old", "cwd": "/p", "message": {"content": "continue"}},
            {"timestamp": "2026-11-15T10:22:00Z", "type": "assistant", "sessionId": "old", "cwd": "/p"},
        ])
        rc, out = self.run_helper(
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T11:00:00Z",
            "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        self.assertEqual(out["resources"][0]["seconds"], 360)
        self.assertEqual(out["resources"][0]["hours"], 0.1)
        self.assertEqual(out["resources"][0]["turn_duration_seconds"], 0)

    def make_stub_project(self):
        project = self.work / "project"
        (project / "canonical").mkdir(parents=True)
        (project / "evidence").mkdir()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n"
            "  - id: actor.ai.claude.executor\n"
            "    kind: ai_agent\n"
            "    display_name: Claude executor\n"
            "    resource_ref: {kind: resource, id: resource.ai.agent.executor}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resources.yaml").write_text(
            "resources:\n"
            "  - id: resource.ai.agent.executor\n"
            "    class: ai_agent\n"
            "    name: Claude executor\n",
            encoding="utf-8",
        )
        (project / "evidence" / "evidence.yaml").write_text("evidence:\n  - id: evidence.test\n", encoding="utf-8")
        (project / "canonical" / "resource_demands.yaml").write_text(
            "resource_demands:\n"
            "  - id: demand.executor\n"
            "    task_ref: {kind: task, id: task.test}\n"
            "    resource_ref: {kind: resource, id: resource.ai.agent.executor}\n"
            "    resource_class: ai_agent\n"
            "    status: active\n",
            encoding="utf-8",
        )
        stub = self.work / "replan-stub"
        stub.write_text(
            "#!/bin/sh\n"
            "printf '%s\\n' '{\"ok\":true,\"result\":{\"new_planned_start\":\"2026-11-15\",\"new_planned_finish\":\"2026-11-15\"},\"errors\":[]}'\n",
            encoding="utf-8",
        )
        stub.chmod(stub.stat().st_mode | stat.S_IXUSR)
        return project, stub

    def test_task_window_and_suggested_commands(self):
        project, stub = self.make_stub_project()
        transcript = self.write_jsonl("claude.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:30:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        codex = self.write_jsonl("codex.jsonl", [
            self.codex_meta("2026-11-15T11:00:00Z", "c"),
            {"timestamp": "2026-11-15T11:04:00Z", "type": "event_msg"},
        ])
        rc, out = self.run_helper(
            "--project", project, "--task", "task.test", "--claude", transcript, "--codex", codex,
            env={"REPLAN_BIN": str(stub)},
        )
        self.assertEqual(rc, 0)
        self.assertEqual(out["window"]["source"], "task_actual_window_dates")
        self.assertIn("window_from_dates", out["warning_codes"])
        self.assertEqual(len(out["suggested_commands"]), 1)
        command = out["suggested_commands"][0]
        self.assertIn("--demand demand.executor", command)
        self.assertIn("--amount 0.25", command)
        self.assertIn("--actor <actor-id>", command)
        self.assertEqual(out["resources"][0]["resource_source"], "task_demand")
        self.assertIn("resource_unresolved", out["warning_codes"])

    def test_auto_picks_single_ai_agent_demand_for_claude(self):
        project, _ = self.make_stub_project()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n  - id: actor.ai.worker\n    kind: ai_agent\n    display_name: General agent\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("single-ai.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:30:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertEqual(out["resources"][0]["resource_id"], "resource.ai.agent.executor")
        self.assertEqual(out["resources"][0]["resource_source"], "task_demand")
        self.assertEqual(len(out["suggested_commands"]), 1)

    def test_auto_picks_claude_and_codex_demands_linked_from_actors(self):
        project, _ = self.make_stub_project()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n"
            "  - id: actor.ai.claude.stream\n"
            "    kind: ai_agent\n"
            "    display_name: Claude stream\n"
            "    resource_ref: {kind: resource, id: resource.ai.claude.stream}\n"
            "  - id: actor.ai.codex.stream\n"
            "    kind: ai_agent\n"
            "    display_name: Codex stream\n"
            "    resource_ref: {kind: resource, id: resource.ai.codex.stream}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resources.yaml").write_text(
            "resources:\n"
            "  - {id: resource.ai.claude.stream, class: ai_agent, name: Claude stream}\n"
            "  - {id: resource.ai.codex.stream, class: ai_agent, name: Codex stream}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resource_demands.yaml").write_text(
            "resource_demands:\n"
            "  - {id: demand.claude, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.claude.stream}, resource_class: ai_agent, status: consumed}\n"
            "  - {id: demand.codex, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.codex.stream}, resource_class: ai_agent, status: active}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        claude = self.write_jsonl("linked-claude.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        codex = self.write_jsonl("linked-codex.jsonl", [
            self.codex_meta("2026-11-15T10:10:00Z", "c"),
            {"timestamp": "2026-11-15T10:15:00Z", "type": "event_msg"},
        ])
        rc, out = self.run_helper(
            "--project", project, "--task", "task.test", "--claude", claude, "--codex", codex,
        )
        self.assertEqual(rc, 0)
        self.assertEqual(
            [(item["resource_id"], item["resource_source"]) for item in out["resources"]],
            [("resource.ai.claude.stream", "task_demand"), ("resource.ai.codex.stream", "task_demand")],
        )
        self.assertEqual(len(out["suggested_commands"]), 2)
        self.assertIn("demand_closed", out["warning_codes"])

    def test_claude_fallback_excludes_codex_linked_demand(self):
        project, _ = self.make_stub_project()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n"
            "  - id: actor.ai.codex.local\n"
            "    kind: ai_agent\n"
            "    display_name: Codex CLI agent\n"
            "    resource_ref: {kind: resource, id: resource.ai.codex.local}\n"
            "  - id: actor.ai.agent.executor\n"
            "    kind: ai_agent\n"
            "    display_name: AI execution agent\n"
            "    resource_ref: {kind: resource, id: resource.ai.agent.executor}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resources.yaml").write_text(
            "resources:\n"
            "  - {id: resource.ai.codex.local, class: ai_agent, name: Codex CLI agent}\n"
            "  - {id: resource.ai.agent.executor, class: ai_agent, name: AI execution agent}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resource_demands.yaml").write_text(
            "resource_demands:\n"
            "  - {id: demand.codex, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.codex.local}, resource_class: ai_agent, status: active}\n"
            "  - {id: demand.executor, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.agent.executor}, resource_class: ai_agent, status: active}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("executor-claude.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertEqual(out["resources"][0]["resource_id"], "resource.ai.agent.executor")
        self.assertEqual(out["resources"][1]["resource_id"], "resource.ai.codex.local")
        self.assertEqual(len(out["suggested_commands"]), 1)
        self.assertIn("--demand demand.executor", out["suggested_commands"][0])

    def test_unmeasured_ambiguous_codex_warns_without_blocking_claude(self):
        project, _ = self.make_stub_project()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n"
            "  - {id: actor.ai.claude.main, kind: ai_agent, display_name: Claude main, resource_ref: {kind: resource, id: resource.ai.claude}}\n"
            "  - {id: actor.ai.codex.one, kind: ai_agent, display_name: Codex one, resource_ref: {kind: resource, id: resource.ai.codex.one}}\n"
            "  - {id: actor.ai.codex.two, kind: ai_agent, display_name: Codex two, resource_ref: {kind: resource, id: resource.ai.codex.two}}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resources.yaml").write_text(
            "resources:\n"
            "  - {id: resource.ai.claude, class: ai_agent, name: Claude}\n"
            "  - {id: resource.ai.codex.one, class: ai_agent, name: Codex one}\n"
            "  - {id: resource.ai.codex.two, class: ai_agent, name: Codex two}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resource_demands.yaml").write_text(
            "resource_demands:\n"
            "  - {id: demand.claude, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.claude}, resource_class: ai_agent, status: active}\n"
            "  - {id: demand.codex.one, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.codex.one}, resource_class: ai_agent, status: active}\n"
            "  - {id: demand.codex.two, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.codex.two}, resource_class: ai_agent, status: active}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("claude-only.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertIn("resource_ambiguous", out["warning_codes"])
        self.assertEqual(out["resources"][1]["resource_id"], "")
        self.assertEqual(len(out["suggested_commands"]), 1)
        self.assertIn("--demand demand.claude", out["suggested_commands"][0])

    def test_multiple_demands_for_same_resource_are_not_resolution_ambiguity(self):
        project, _ = self.make_stub_project()
        demands = project / "canonical" / "resource_demands.yaml"
        demands.write_text(
            demands.read_text(encoding="utf-8")
            + "  - {id: demand.executor.second, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.agent.executor}, resource_class: ai_agent, status: consumed}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("same-resource.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertNotIn("resource_ambiguous", out["warning_codes"])
        self.assertIn("multiple_demands_for_resource", out["warning_codes"])
        self.assertEqual(len(out["suggested_commands"]), 1)

    def test_auto_refuses_ambiguous_resource_demands(self):
        project, _ = self.make_stub_project()
        (project / "canonical" / "actors.yaml").write_text(
            "actors:\n"
            "  - {id: actor.ai.claude.one, kind: ai_agent, display_name: Claude one, resource_ref: {kind: resource, id: resource.ai.one}}\n"
            "  - {id: actor.ai.claude.two, kind: ai_agent, display_name: Claude two, resource_ref: {kind: resource, id: resource.ai.two}}\n",
            encoding="utf-8",
        )
        (project / "canonical" / "resource_demands.yaml").write_text(
            "resource_demands:\n"
            "  - {id: demand.one, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.one}, resource_class: ai_agent, status: active}\n"
            "  - {id: demand.two, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.two}, resource_class: ai_agent, status: consumed}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("ambiguous-claude.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 2)
        self.assertEqual(out["error_codes"], ["resource_ambiguous"])
        self.assertIn("demand.one (resource.ai.one)", out["errors"][0]["message"])
        self.assertIn("demand.two (resource.ai.two)", out["errors"][0]["message"])

    def test_resource_flag_overrides_auto_selection(self):
        project, _ = self.make_stub_project()
        resources = project / "canonical" / "resources.yaml"
        resources.write_text(
            resources.read_text(encoding="utf-8")
            + "  - {id: resource.ai.override, class: ai_agent, name: Override}\n",
            encoding="utf-8",
        )
        demands = project / "canonical" / "resource_demands.yaml"
        demands.write_text(
            demands.read_text(encoding="utf-8")
            + "  - {id: demand.override, task_ref: {kind: task, id: task.test}, resource_ref: {kind: resource, id: resource.ai.override}, resource_class: ai_agent, status: active}\n",
            encoding="utf-8",
        )
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        rc, out = self.run_helper(
            "--project", project, "--task", "task.test", "--claude-resource", "resource.ai.override",
        )
        self.assertEqual(rc, 0)
        self.assertEqual(out["resources"][0]["resource_id"], "resource.ai.override")
        self.assertEqual(out["resources"][0]["resource_source"], "flag")

    def test_resource_not_in_project_warns_for_flag(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        rc, out = self.run_helper(
            "--project", project, "--task", "task.test", "--claude-resource", "resource.ai.missing",
        )
        self.assertEqual(rc, 0)
        self.assertIn("resource_not_in_project", out["warning_codes"])
        self.assertEqual(out["resources"][0]["resource_source"], "flag")

    def test_explicit_window_uses_legacy_default_resources(self):
        rc, out = self.run_helper(
            "--from", "2026-11-15T10:00:00Z", "--to", "2026-11-15T11:00:00Z",
        )
        self.assertEqual(rc, 0)
        self.assertEqual(
            [(item["resource_id"], item["resource_source"]) for item in out["resources"]],
            [("resource.ai.agent.executor", "default"), ("resource.ai.codex.local", "default")],
        )

    def write_status_records(self, project, records, task="task.test", name="audit.synthetic.jsonl"):
        audit = project / "audit" / name
        audit.parent.mkdir(parents=True, exist_ok=True)
        with audit.open("w", encoding="utf-8") as stream:
            for timestamp, status in records:
                stream.write(json.dumps({
                    "id": f"audit.{task}.{status}.{timestamp}", "occurred_at": timestamp,
                    "operation": "actual_add_status", "target_ref": {"kind": "task", "id": task},
                    "new_status": status,
                }) + "\n")

    def test_overlapping_task_windows_warn_and_from_narrows(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T11:00:00Z", "review_ready"),
        ])
        # The next task started at 10:40 while this one waited for its gate.
        self.write_status_records(project, [
            ("2026-11-15T10:40:00Z", "in_progress"),
            ("2026-11-15T12:00:00Z", "review_ready"),
        ], task="task.next", name="audit.next.jsonl")
        # A task that finished before the window does not overlap.
        self.write_status_records(project, [
            ("2026-11-15T08:00:00Z", "in_progress"),
            ("2026-11-15T09:00:00Z", "review_ready"),
        ], task="task.before", name="audit.before.jsonl")
        transcript = self.write_jsonl("overlap.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:04:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
            {"timestamp": "2026-11-15T10:50:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "next"}},
            {"timestamp": "2026-11-15T10:53:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertIn("overlapping_task_windows", out["warning_codes"])
        self.assertEqual(out["window"]["overlapping_tasks"], [
            {
                "task": "task.next", "from": "2026-11-15T10:40:00.000Z",
                "to": "2026-11-15T11:00:00.000Z", "seconds": 1200,
                "kind": "close", "window_hours": 1.333333,
            },
        ])
        self.assertEqual(out["resources"][0]["seconds"], 420)

        rc, narrowed = self.run_helper(
            "--project", project, "--task", "task.test", "--to", "2026-11-15T10:40:00Z", "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        self.assertNotIn("overlapping_task_windows", narrowed["warning_codes"])
        self.assertEqual(narrowed["resources"][0]["seconds"], 240)

        rc, from_narrowed = self.run_helper(
            "--project", project, "--task", "task.test", "--from", "2026-11-15T10:45:00Z", "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        self.assertEqual(from_narrowed["window"]["source"], "status_records_narrowed")
        self.assertEqual(from_narrowed["window"]["from"], "2026-11-15T10:45:00.000Z")
        self.assertEqual(from_narrowed["resources"][0]["seconds"], 180)

    def test_close_and_weeks_long_open_overlaps_are_reported_separately(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T11:00:00Z", "review_ready"),
        ])
        self.write_status_records(project, [
            ("2026-11-15T09:50:00Z", "in_progress"),
            ("2026-11-15T10:20:00Z", "review_ready"),
        ], task="task.close.one", name="audit.close-one.jsonl")
        self.write_status_records(project, [
            ("2026-11-15T10:40:00Z", "in_progress"),
            ("2026-11-15T11:10:00Z", "review_ready"),
        ], task="task.close.two", name="audit.close-two.jsonl")
        self.write_status_records(
            project, [("2026-10-01T00:00:00Z", "in_progress")],
            task="task.weeks.open", name="audit.weeks-open.jsonl",
        )

        rc, out = self.run_helper("--project", project, "--task", "task.test")
        self.assertEqual(rc, 0)
        warning = next(item for item in out["warnings"] if item["code"] == "overlapping_task_windows")
        info = next(item for item in out["warnings"] if item["code"] == "long_running_task_overlaps")
        self.assertIn("task.close.one", warning["message"])
        self.assertIn("task.close.two", warning["message"])
        self.assertNotIn("task.weeks.open", warning["message"])
        self.assertEqual(warning["severity"], "warning")
        self.assertIn("task.weeks.open", info["message"])
        self.assertNotIn("task.close.one", info["message"])
        self.assertNotIn("task.close.two", info["message"])
        self.assertEqual(info["severity"], "info")
        overlaps = {item["task"]: item for item in out["window"]["overlapping_tasks"]}
        self.assertEqual(
            {task: item["kind"] for task, item in overlaps.items()},
            {"task.close.one": "close", "task.close.two": "close", "task.weeks.open": "long_running"},
        )
        self.assertEqual(overlaps["task.close.one"]["window_hours"], 0.5)
        self.assertEqual(overlaps["task.close.two"]["window_hours"], 0.5)
        self.assertGreater(overlaps["task.weeks.open"]["window_hours"], 24)

    def test_only_long_running_overlap_emits_info_without_overlap_warning(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T11:00:00Z", "review_ready"),
        ])
        self.write_status_records(project, [
            ("2026-11-14T10:00:00Z", "in_progress"),
            ("2026-11-15T12:00:00Z", "review_ready"),
        ], task="task.long", name="audit.long.jsonl")

        rc, out = self.run_helper("--project", project, "--task", "task.test")
        self.assertEqual(rc, 0)
        self.assertNotIn("overlapping_task_windows", out["warning_codes"])
        self.assertIn("long_running_task_overlaps", out["warning_codes"])
        info = next(item for item in out["warnings"] if item["code"] == "long_running_task_overlaps")
        self.assertEqual(info["severity"], "info")
        self.assertEqual(out["window"]["overlapping_tasks"][0]["kind"], "long_running")
        self.assertEqual(out["window"]["overlapping_tasks"][0]["window_hours"], 26)

    def test_long_running_overlap_threshold_boundaries(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T00:00:00Z", "in_progress"),
            ("2026-11-15T04:48:00Z", "review_ready"),
        ])
        # Exactly 5x the 4.8-hour measured window and exactly 24 hours long.
        self.write_status_records(project, [
            ("2026-11-14T04:48:00Z", "in_progress"),
            ("2026-11-15T04:48:00Z", "review_ready"),
        ], task="task.exact", name="audit.exact.jsonl")
        # More than 5x a separate four-hour window, but still under 24 hours.
        self.write_status_records(project, [
            ("2026-11-14T05:18:00Z", "in_progress"),
            ("2026-11-15T04:48:00Z", "review_ready"),
        ], task="task.under-day", name="audit.under-day.jsonl")
        self.write_status_records(project, [
            ("2026-11-15T00:00:00Z", "in_progress"),
            ("2026-11-15T04:00:00Z", "review_ready"),
        ], task="task.short", name="audit.short.jsonl")

        rc, out = self.run_helper("--project", project, "--task", "task.test")
        self.assertEqual(rc, 0)
        overlaps = {item["task"]: item for item in out["window"]["overlapping_tasks"]}
        self.assertEqual(overlaps["task.exact"]["kind"], "long_running")
        self.assertEqual(overlaps["task.exact"]["window_hours"], 24)

        rc, out = self.run_helper("--project", project, "--task", "task.short")
        self.assertEqual(rc, 0)
        overlaps = {item["task"]: item for item in out["window"]["overlapping_tasks"]}
        self.assertEqual(overlaps["task.under-day"]["kind"], "close")
        self.assertEqual(overlaps["task.under-day"]["window_hours"], 23.5)

    def test_task_window_uses_status_timestamps_and_to_override(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
            ("2026-11-15T11:00:00Z", "accepted"),
        ])
        transcript = self.write_jsonl("status-window.jsonl", [
            {"timestamp": "2026-11-15T09:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "other task"}},
            {"timestamp": "2026-11-15T10:05:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:10:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
            {"timestamp": "2026-11-15T11:30:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertEqual(out["window"], {"from": "2026-11-15T10:00:00.000Z", "to": "2026-11-15T10:30:00.000Z", "source": "status_records", "overlapping_tasks": []})
        self.assertNotIn("window_from_dates", out["warning_codes"])

        rc, overridden = self.run_helper(
            "--project", project, "--task", "task.test", "--to", "2026-11-15T10:08:00Z", "--claude", transcript,
        )
        self.assertEqual(rc, 0)
        self.assertEqual(overridden["window"]["to"], "2026-11-15T10:08:00.000Z")
        self.assertEqual(overridden["resources"][0]["seconds"], 180)

    def test_open_task_window_ends_now_with_warning(self):
        project, _ = self.make_stub_project()
        self.write_status_records(project, [("2026-11-15T10:00:00Z", "in_progress")])
        rc, out = self.run_helper("--project", project, "--task", "task.test")
        self.assertEqual(rc, 0)
        self.assertEqual(out["window"]["source"], "status_records")
        self.assertIn("window_open", out["warning_codes"])

    def test_closed_demand_warns_and_still_suggests_consumption(self):
        project, _ = self.make_stub_project()
        demands = project / "canonical" / "resource_demands.yaml"
        demands.write_text(demands.read_text(encoding="utf-8").replace("status: active", "status: consumed"), encoding="utf-8")
        self.write_status_records(project, [
            ("2026-11-15T10:00:00Z", "in_progress"),
            ("2026-11-15T10:30:00Z", "review_ready"),
        ])
        transcript = self.write_jsonl("closed-demand.jsonl", [
            {"timestamp": "2026-11-15T10:00:00Z", "type": "user", "sessionId": "s", "cwd": "/p", "message": {"content": "go"}},
            {"timestamp": "2026-11-15T10:30:00Z", "type": "assistant", "sessionId": "s", "cwd": "/p"},
        ])
        rc, out = self.run_helper("--project", project, "--task", "task.test", "--claude", transcript)
        self.assertEqual(rc, 0)
        self.assertIn("demand_closed", out["warning_codes"])
        self.assertNotIn("no_demand_for_resource", out["warning_codes"])
        self.assertEqual(len(out["suggested_commands"]), 1)
        self.assertIn("--demand demand.executor", out["suggested_commands"][0])

    def test_missing_window_is_usage_error(self):
        rc, out = self.run_helper()
        self.assertEqual(rc, 8)
        self.assertEqual(out["error_codes"], ["missing_window"])
        self.assertEqual(out["mode"], "invalid")


if __name__ == "__main__":
    unittest.main()
