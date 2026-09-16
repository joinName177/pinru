import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import openpyxl


EXPORTER = Path(__file__).resolve().parents[1] / "export_pairwise.py"


class ExportPairwiseTests(unittest.TestCase):
    def test_export_pairwise_writes_shared_a_b_and_gsb_columns(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            payload = {
                "projectName": "Pairwise Demo",
                "submitter": "Reviewer",
                "submittedAt": "2026-09-17",
                "issues": [],
                "cases": [{
                    "taskId": "task-1",
                    "taskName": "加法题",
                    "taskType": "0-1代码生成",
                    "promptDifficulty": "简单",
                    "initialSha": "1" * 40,
                    "snapshotUrl": "https://github.com/example/repo/commit/" + "1" * 40,
                    "pairwise": {
                        "prompt": "实现加法功能",
                        "harness": "Codex",
                        "harnessVersion": "1.2.3",
                        "os": "MacOS/Linux",
                        "environment": "本地仓库",
                        "runA": self.pairwise_run("A", "a"),
                        "runB": self.pairwise_run("B", "b"),
                        "reviews": [{
                            "id": "review-1", "status": "ready", "conclusion": "A_better",
                            "reason": "A 保留返回值；B 删除返回值，因此 A 更完整。",
                            "sourceHashA": "source-a", "sourceHashB": "source-b",
                        }],
                        "notes": "人工复核完成",
                    },
                    "currentPairwiseReview": {
                        "conclusion": "A_better",
                        "reason": "A 保留返回值；B 删除返回值，因此 A 更完整。",
                    },
                }],
            }
            source = root / "input.json"
            source.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
            output = root / "output"
            proc = subprocess.run(
                [sys.executable, str(EXPORTER), "--input", str(source), "--output", str(output)],
                text=True, capture_output=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            result = json.loads(proc.stdout)
            self.assertEqual(result["rows"], 1)
            sheet = openpyxl.load_workbook(result["outputPath"]).active
            self.assertEqual([cell.value for cell in sheet[1]][:6], [
                "User Prompt", "初始环境快照", "A-SessionID", "A-产物快照", "B-SessionID", "B-产物快照",
            ])
            self.assertEqual(sheet["A2"].value, "实现加法功能")
            self.assertEqual(sheet["Q2"].value, "A 更好")

    @staticmethod
    def pairwise_run(side, sha_char):
        return {
            "side": side,
            "branch": side,
            "sessionId": "session-" + side.lower(),
            "tracePath": "/evidence/" + side + "/session.jsonl",
            "turnCount": 1,
            "deliverableSha": sha_char * 40,
            "deliverableUrl": "https://github.com/example/repo/commit/" + sha_char * 40,
            "videoStatus": "ready",
            "videoUrl": "https://example.com/" + side.lower() + ".mp4",
        }


if __name__ == "__main__":
    unittest.main()
