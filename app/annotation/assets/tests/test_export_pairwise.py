import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import openpyxl


EXPORTER = Path(__file__).resolve().parents[1] / "export_pairwise.py"


class ExportPairwiseTests(unittest.TestCase):
    def test_export_pairwise_writes_shared_a_b_and_gsb_columns_without_third_party_runtime(self):
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
                    "promptDifficulty": "困难",
                    "initialSha": "1" * 40,
                    "snapshotUrl": "https://github.com/example/repo/commit/" + "1" * 40,
                    "pairwise": {
                        "prompt": "实现加法功能",
                        "language": "Python, pytest",
                        "harness": "Claude Code",
                        "harnessVersion": "1.2.3",
                        "os": "MacOS/Linux",
                        "environment": "本地仓库",
                        "runA": self.pairwise_run("A", "a"),
                        "runB": self.pairwise_run("B", "b"),
                        "reviews": [{
                            "id": "review-1", "status": "ready", "conclusion": "A_better",
                            "reason": "A 保留返回值；B 删除返回值，因此 A 更完整。",
                            "aCompletenessScore": 5,
                            "aCompletenessDescription": "A 已交付需求中的返回值处理，产物可以完成预期调用。",
                            "bCompletenessScore": 3,
                            "bCompletenessDescription": "B 的提交缺少返回值处理，交付结果无法覆盖完整调用链。",
                            "sourceHashA": "source-a", "sourceHashB": "source-b",
                        }],
                        "notes": "人工复核完成",
                        "validity": "有效",
                    },
                    "currentPairwiseReview": {
                        "conclusion": "A_better",
                        "reason": "A 保留返回值；B 删除返回值，因此 A 更完整。",
                        "aCompletenessScore": 5,
                        "aCompletenessDescription": "A 已交付需求中的返回值处理，产物可以完成预期调用。",
                        "bCompletenessScore": 3,
                        "bCompletenessDescription": "B 的提交缺少返回值处理，交付结果无法覆盖完整调用链。",
                    },
                }],
            }
            source = root / "input.json"
            source.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
            output = root / "output"
            proc = subprocess.run(
                [sys.executable, "-S", str(EXPORTER), "--input", str(source), "--output", str(output)],
                text=True, capture_output=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            result = json.loads(proc.stdout)
            self.assertEqual(result["rows"], 1)
            sheet = openpyxl.load_workbook(result["outputPath"]).active
            expected_headers = [
                "User Prompt", "任务类型", "任务难度", "语言/框架", "Harness", "Harness 版本",
                "操作系统", "环境可复现等级", "初始环境快照", "A-SessionID", "A-轨迹文件",
                "A-产物快照", "A-运行录屏", "B-SessionID", "B-轨迹文件", "B-产物快照",
                "B-运行录屏", "A-交付完整性", "A-交付完整性描述", "B-交付完整性", "B-交付完整性描述",
                "GSB 结论", "GSB 理由", "有效性", "备注",
            ]
            self.assertEqual([cell.value for cell in sheet[1]][:len(expected_headers)], expected_headers)
            self.assertEqual(sheet["A2"].value, "实现加法功能")
            self.assertEqual(sheet["R2"].value, 5)
            self.assertEqual(sheet["S2"].value, "A 已交付需求中的返回值处理，产物可以完成预期调用。")
            self.assertEqual(sheet["T2"].value, 3)
            self.assertEqual(sheet["U2"].value, "B 的提交缺少返回值处理，交付结果无法覆盖完整调用链。")
            self.assertEqual(sheet["V2"].value, "A 更好")
            self.assertEqual(sheet["D2"].value, "Python, pytest")
            self.assertEqual(sheet["X2"].value, "有效")

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
