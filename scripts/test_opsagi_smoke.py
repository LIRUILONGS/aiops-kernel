#!/usr/bin/env python3
"""Tests for scripts/opsagi_smoke.py, without a backend or a model.

    python scripts/test_opsagi_smoke.py

Every check is exercised in both directions: a healthy exit passes it, and a
payload with that one property broken fails it. A check that cannot fail is the
failure mode this file exists to catch.
"""

import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import opsagi_smoke


def diagnosis(**overrides):
    payload = {
        "root_cause": "caddy 在 13:02 因端口占用退出，systemd 重启后没有重新绑定",
        "evidence": [
            {
                "source": "containers",
                "query": "ops_containers container=caddy",
                "timestamp": "2026-11-06T13:02:41+08:00",
                "excerpt": "restarts 3, exit_code 1",
            },
            {
                "source": "logs",
                "query": "ops_logs source=caddy lines=50",
                "timestamp": "2026-11-06T13:02:44+08:00",
                "excerpt": "bind: address already in use",
            },
        ],
        "confidence": 0.6,
        "blast_radius": "只有 lab 主机上的 80 端口不可用，持续 41 秒",
        "recommendation": {"action": "先确认 80 端口被谁占用，再决定是否重启 caddy"},
        "needs_human": True,
    }
    payload.update(overrides)
    return payload


def optimization(**overrides):
    payload = {
        "bottleneck": "磁盘写入被 caddy 日志与容器层同时打满",
        "baseline": "13:50 采集：load1 0.9，写入 12MB/s",
        "proposal": "把 caddy 访问日志改为按天轮转，保留 7 天",
        "expected_gain": "写入降到 3MB/s",
        "observed_gain": "",
        "rollback": "恢复原 Caddyfile 并 restart caddy",
        "verdict": "unverified",
        "evidence": [
            {"source": "host", "query": "ops_baseline label=before", "timestamp": "2026-11-06T13:50:11+08:00", "excerpt": "load1=0.9"},
            {"source": "containers", "query": "ops_containers", "timestamp": "2026-11-06T13:50:20+08:00", "excerpt": "caddy up, restarts 518"},
        ],
    }
    payload.update(overrides)
    return payload


def call(name, payload):
    return {"args": json.dumps(payload, ensure_ascii=False), "name": name, "result": "ok"}


class CheckFlow(unittest.TestCase):
    def test_a_guarded_diagnosis_passes(self):
        self.assertEqual(opsagi_smoke.check_flow("finished", [call("diagnosis", diagnosis())], "diagnosis"), [])

    def test_a_guarded_optimization_passes(self):
        self.assertEqual(
            opsagi_smoke.check_flow("finished", [call("optimization_result", optimization())], "optimization"), []
        )

    def test_a_flow_that_answered_and_waits_is_settled(self):
        self.assertEqual(opsagi_smoke.check_flow("waiting", [call("diagnosis", diagnosis())], "diagnosis"), [])

    def test_a_failed_flow_fails(self):
        failures = opsagi_smoke.check_flow("failed", [call("diagnosis", diagnosis())], "diagnosis")

        self.assertIn("flow failed", failures)

    def test_a_flow_still_planning_fails(self):
        failures = opsagi_smoke.check_flow("running", [call("diagnosis", diagnosis())], "diagnosis")

        self.assertTrue(any("not finished or waiting" in failure for failure in failures), failures)

    def test_a_flow_without_an_exit_fails(self):
        failures = opsagi_smoke.check_flow("finished", [call("ops_system", {"host": "lab"})], "diagnosis")

        self.assertTrue(any("no diagnosis call" in failure for failure in failures), failures)

    def test_the_last_exit_is_the_one_that_must_hold(self):
        calls = [call("diagnosis", diagnosis()), call("diagnosis", diagnosis(confidence=2.0))]
        failures = opsagi_smoke.check_flow("finished", calls, "diagnosis")

        self.assertEqual(len(failures), 1, failures)
        self.assertIn("outside 0..1", failures[0])

    def test_a_refusal_that_was_repaired_passes(self):
        calls = [
            dict(call("diagnosis", diagnosis(blast_radius="")), status="failed"),
            call("diagnosis", diagnosis()),
        ]

        self.assertEqual(opsagi_smoke.check_flow("finished", calls, "diagnosis"), [])

    def test_a_last_exit_that_was_refused_fails(self):
        calls = [call("diagnosis", diagnosis()), dict(call("diagnosis", diagnosis()), status="failed")]
        failures = opsagi_smoke.check_flow("finished", calls, "diagnosis")

        self.assertTrue(any("last diagnosis call ended as 'failed'" in failure for failure in failures), failures)

    def test_two_healthy_exits_pass(self):
        calls = [call("diagnosis", diagnosis()), call("diagnosis", diagnosis())]

        self.assertEqual(opsagi_smoke.check_flow("finished", calls, "diagnosis"), [])

    def test_args_that_are_not_json_fail(self):
        failures = opsagi_smoke.check_flow("finished", [{"args": "{not json", "name": "diagnosis"}], "diagnosis")

        self.assertTrue(any("are not JSON" in failure for failure in failures), failures)

    def test_args_already_decoded_are_accepted(self):
        failures = opsagi_smoke.check_flow("finished", [{"args": diagnosis(), "name": "diagnosis"}], "diagnosis")

        self.assertEqual(failures, [])


class CheckDiagnosis(unittest.TestCase):
    def test_every_required_field_is_checked(self):
        for field in ("root_cause", "blast_radius"):
            with self.subTest(field=field):
                self.assertTrue(any(field in failure for failure in opsagi_smoke.check_diagnosis(diagnosis(**{field: " "}))))

    def test_a_recommendation_without_an_action_fails(self):
        failures = opsagi_smoke.check_diagnosis(diagnosis(recommendation={"action": ""}))

        self.assertTrue(any("recommendation.action" in failure for failure in failures), failures)

    def test_a_confidence_outside_the_unit_interval_fails(self):
        failures = opsagi_smoke.check_diagnosis(diagnosis(confidence=1.4))

        self.assertTrue(any("outside 0..1" in failure for failure in failures), failures)

    def test_one_piece_of_evidence_fails(self):
        item = diagnosis()["evidence"][0]
        failures = opsagi_smoke.check_diagnosis(diagnosis(evidence=[item]))

        self.assertTrue(any("want at least 2" in failure for failure in failures), failures)

    def test_evidence_missing_a_field_fails(self):
        evidence = diagnosis()["evidence"]
        evidence[1] = dict(evidence[1], excerpt="")

        failures = opsagi_smoke.check_diagnosis(diagnosis(evidence=evidence))

        self.assertTrue(any("evidence[1].excerpt is empty" in failure for failure in failures), failures)


class CheckOptimization(unittest.TestCase):
    def test_every_required_field_is_checked(self):
        for field in ("bottleneck", "baseline", "proposal", "expected_gain", "rollback"):
            with self.subTest(field=field):
                self.assertTrue(
                    any(field in failure for failure in opsagi_smoke.check_optimization(optimization(**{field: ""})))
                )

    def test_an_unknown_verdict_fails(self):
        failures = opsagi_smoke.check_optimization(optimization(verdict="probably-fine"))

        self.assertTrue(any("is not one of" in failure for failure in failures), failures)

    def test_a_measured_verdict_without_a_number_fails(self):
        failures = opsagi_smoke.check_optimization(optimization(verdict="improved", observed_gain=""))

        self.assertTrue(any("needs a number" in failure for failure in failures), failures)

    def test_a_measured_verdict_with_a_number_passes(self):
        failures = opsagi_smoke.check_optimization(optimization(verdict="improved", observed_gain="load1 0.9 → 0.2"))

        self.assertEqual(failures, [])

    def test_unverified_must_not_carry_a_measurement(self):
        failures = opsagi_smoke.check_optimization(optimization(verdict="unverified", observed_gain="load1 0.9 → 0.2"))

        self.assertTrue(any("that is not a measurement" in failure for failure in failures), failures)


class Toolcalls(unittest.TestCase):
    class Client:
        def __init__(self, rows):
            self.rows = rows

        def call(self, _path):
            return {"data": {"toolcalls": self.rows}}

    def test_rows_come_back_in_the_order_they_were_written(self):
        rows = [
            {"id": 306, "name": "diagnosis", "status": "finished"},
            {"id": 304, "name": "diagnosis", "status": "failed"},
        ]

        calls = opsagi_smoke.toolcalls(Toolcalls.Client(rows), 20)

        self.assertEqual([304, 306], [call["id"] for call in calls])

    def test_a_payload_that_is_not_a_list_is_no_calls(self):
        self.assertEqual(opsagi_smoke.toolcalls(Toolcalls.Client(None), 20), [])


class ReadPayload(unittest.TestCase):
    def test_a_dict_is_returned_as_is(self):
        payload = diagnosis()

        self.assertEqual(opsagi_smoke.read_payload({"args": payload}), payload)

    def test_empty_and_missing_args_are_none(self):
        self.assertIsNone(opsagi_smoke.read_payload({"args": ""}))
        self.assertIsNone(opsagi_smoke.read_payload({}))
        self.assertIsNone(opsagi_smoke.read_payload({"args": "[1, 2]"}))


if __name__ == "__main__":
    unittest.main(verbosity=2)
