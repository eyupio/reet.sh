"""Credential-free tests of the probe's interpretation, without starting Cline."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("cline_probe", Path(__file__).with_name("probe-cline-tui-resume.py"))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


class VerdictTests(unittest.TestCase):
    def report(self):
        return {
            "chatIdHasReetShape": True,
            "tui": {"exitCode": None, "stillRunningAfterSeconds": True},
            "historyBefore": {"exit": 0, "listsChatId": True},
            "historyAfter": {"exit": 0, "listsChatId": True, "newIds": []},
            "loadAfterTui": {"ok": True},
        }

    def test_refusal_and_failed_history_cannot_prove_resume(self):
        r = self.report()
        r["tui"] = {"exitCode": 1, "mentionsRefusal": True}
        r["historyBefore"]["exit"] = 1
        r["historyAfter"]["exit"] = 1
        text = " ".join(probe.verdict(r))
        self.assertIn("do NOT add", text)
        self.assertIn("unknown", text)
        self.assertNotIn("it kept", text)
        self.assertNotIn("`--id` is accepted", text)

    def test_missing_history_is_unknown(self):
        text = " ".join(probe.verdict({"tui": {"exitCode": 1}}))
        self.assertIn("incomplete", text)
        self.assertNotIn("it kept", text)

    def test_empty_successful_history_does_not_prove_binding(self):
        r = self.report()
        r["historyAfter"]["listsChatId"] = False
        self.assertIn("continuity is unverified", " ".join(probe.verdict(r)))

    def test_no_new_ids_and_successful_load_still_need_content_check(self):
        text = " ".join(probe.verdict(self.report()))
        self.assertIn("no new task id was observed", text)
        self.assertIn("before enabling switching", text)
        self.assertIn("does not establish transcript continuity", text)

    def test_observed_new_ids_warn_of_possible_fork(self):
        r = self.report()
        r["historyAfter"]["newIds"] = ["1234567890123_abcde_cli"]
        self.assertIn("may fork", " ".join(probe.verdict(r)))


if __name__ == "__main__":
    unittest.main()
