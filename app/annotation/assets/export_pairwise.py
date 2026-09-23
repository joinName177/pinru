#!/usr/bin/env python3
import argparse
import json
from pathlib import Path
import sys
import xml.etree.ElementTree as ET
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo


MAIN_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
NS = "{" + MAIN_NS + "}"
REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
XML_SPACE = "{http://www.w3.org/XML/1998/namespace}space"
ET.register_namespace("", MAIN_NS)
ET.register_namespace("r", REL_NS)

TEMPLATE = Path(__file__).with_name("template.xlsx")
SHEET_PATH = "xl/worksheets/sheet1.xml"
WORKBOOK_PATH = "xl/workbook.xml"
FIXED_ZIP_TIME = (1980, 1, 1, 0, 0, 0)

HEADERS = [
    "User Prompt", "任务类型", "任务难度", "语言/框架", "Harness", "Harness 版本",
    "操作系统", "环境可复现等级", "初始环境快照", "A-SessionID", "A-轨迹文件",
    "A-产物快照", "A-运行录屏", "B-SessionID", "B-轨迹文件", "B-产物快照",
    "B-运行录屏", "A-交付完整性", "A-交付完整性描述", "B-交付完整性", "B-交付完整性描述",
    "GSB 结论", "GSB 理由", "有效性", "备注",
]
CONCLUSIONS = {"A_better": "A 更好", "B_better": "B 更好", "same": "Same"}
WIDTHS = [36, 18, 14, 28, 16, 16, 18, 24, 48, 24, 42, 48, 42, 24, 42, 48, 42, 14, 48, 14, 48, 14, 72, 22, 32]


def current_review(case):
    reviews = (case.get("pairwise") or {}).get("reviews") or []
    return reviews[-1] if reviews else {}


def captured_trace(case, run):
    capture_id = run.get("captureId")
    for capture in case.get("captures") or []:
        if capture.get("id") == capture_id and capture.get("tracePath"):
            return capture["tracePath"]
    return run.get("tracePath", "")


def row(case, payload):
    pairwise = case.get("pairwise") or {}
    run_a = pairwise.get("runA") or {}
    run_b = pairwise.get("runB") or {}
    review = current_review(case)
    return [
        pairwise.get("prompt", ""), case.get("taskType", ""), case.get("promptDifficulty", ""),
        pairwise.get("language", ""),
        pairwise.get("harness", ""), pairwise.get("harnessVersion", ""), pairwise.get("os", ""),
        pairwise.get("environment", ""), case.get("snapshotUrl", ""),
        run_a.get("sessionId", ""), captured_trace(case, run_a), run_a.get("deliverableUrl", ""),
        run_a.get("videoPath") or run_a.get("videoUrl", ""), run_b.get("sessionId", ""),
        captured_trace(case, run_b), run_b.get("deliverableUrl", ""),
        run_b.get("videoPath") or run_b.get("videoUrl", ""),
        review.get("aCompletenessScore", ""), review.get("aCompletenessDescription", ""),
        review.get("bCompletenessScore", ""), review.get("bCompletenessDescription", ""),
        CONCLUSIONS.get(review.get("conclusion"), ""), review.get("reason", ""),
        pairwise.get("validity", ""), pairwise.get("notes", ""),
    ]


def column_name(number):
    result = ""
    while number:
        number, remainder = divmod(number - 1, 26)
        result = chr(65 + remainder) + result
    return result


def inline_cell(row_number, column, value, style):
    reference = column_name(column + 1) + str(row_number)
    cell = ET.Element(NS + "c", {"r": reference, "s": str(style), "t": "inlineStr"})
    inline = ET.SubElement(cell, NS + "is")
    text = ET.SubElement(inline, NS + "t")
    value = str(value)
    if value[:1].isspace() or value[-1:].isspace() or "\n" in value or "\r" in value:
        text.set(XML_SPACE, "preserve")
    text.text = value
    return cell


def number_cell(row_number, column, value, style):
    reference = column_name(column + 1) + str(row_number)
    cell = ET.Element(NS + "c", {"r": reference, "s": str(style)})
    number = ET.SubElement(cell, NS + "v")
    number.text = str(value)
    return cell


def build_sheet(data, data_rows):
    root = ET.fromstring(data)
    sheet_data = root.find(NS + "sheetData")
    sheet_data.clear()

    header = ET.SubElement(sheet_data, NS + "row", {"r": "1", "ht": "36", "customHeight": "1"})
    for column, value in enumerate(HEADERS):
        header.append(inline_cell(1, column, value, 4))

    for row_number, values in enumerate(data_rows, 2):
        xml_row = ET.SubElement(sheet_data, NS + "row", {
            "r": str(row_number), "ht": "90", "customHeight": "1",
        })
        for column, value in enumerate(values):
            if value is not None and value != "":
                if column in (17, 19):
                    xml_row.append(number_cell(row_number, column, value, 6))
                else:
                    xml_row.append(inline_cell(row_number, column, value, 6))

    last_column = column_name(len(HEADERS))
    last_row = max(1, len(data_rows) + 1)
    root.find(NS + "dimension").set("ref", f"A1:{last_column}{last_row}")

    columns = root.find(NS + "cols")
    columns.clear()
    for index, width in enumerate(WIDTHS, 1):
        ET.SubElement(columns, NS + "col", {
            "width": str(width), "customWidth": "1", "min": str(index), "max": str(index),
        })

    auto_filter = root.find(NS + "autoFilter")
    if auto_filter is not None:
        auto_filter.set("ref", f"A1:{last_column}{last_row}")
    validations = root.find(NS + "dataValidations")
    if validations is not None:
        root.remove(validations)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True)


def build_workbook(data_rows, destination):
    if not TEMPLATE.is_file():
        raise FileNotFoundError(f"bundled template is missing: {TEMPLATE}")
    with ZipFile(TEMPLATE, "r") as source:
        entries = [(item.filename, source.read(item.filename)) for item in source.infolist()]

    replacements = {}
    for filename, data in entries:
        if filename == SHEET_PATH:
            replacements[filename] = build_sheet(data, data_rows)
        elif filename == WORKBOOK_PATH:
            root = ET.fromstring(data)
            sheet = root.find(NS + "sheets/" + NS + "sheet")
            sheet.set("name", "Pair-wise GSB")
            defined_name = root.find(NS + "definedNames/" + NS + "definedName")
            if defined_name is not None:
                last_column = column_name(len(HEADERS))
                defined_name.text = f"'Pair-wise GSB'!$A$1:${last_column}${max(1, len(data_rows) + 1)}"
            replacements[filename] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    destination.parent.mkdir(parents=True, exist_ok=True)
    with ZipFile(destination, "w", ZIP_DEFLATED, compresslevel=9) as target:
        for filename, data in entries:
            info = ZipInfo(filename, FIXED_ZIP_TIME)
            info.compress_type = ZIP_DEFLATED
            info.external_attr = 0o600 << 16
            target.writestr(info, replacements.get(filename, data))


def export(payload, output):
    output.mkdir(parents=True, exist_ok=True)
    cases = payload.get("cases") or []
    data_rows = [row(case, payload) for case in cases]
    workbook_path = (output / "pairwise-gsb.xlsx").resolve()
    build_workbook(data_rows, workbook_path)

    issues = payload.get("issues") or []
    report_path = (output / "pairwise-report.md").resolve()
    lines = ["# Pair-wise GSB 导出报告", "", f"- 题目数：{len(payload.get('cases') or [])}", f"- 待补项：{len(issues)}"]
    if issues:
        lines.extend(["", "## 待补材料", ""] + [f"- {item}" for item in issues])
    report_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return {
        "outputPath": str(workbook_path),
        "reportPath": str(report_path),
        "rows": len(cases),
        "issues": issues,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--draft", action="store_true")
    args = parser.parse_args()
    try:
        payload = json.loads(Path(args.input).read_text(encoding="utf-8"))
        print(json.dumps(export(payload, Path(args.output)), ensure_ascii=False))
    except Exception as exc:
        print(json.dumps({"outputPath": "", "reportPath": "", "rows": 0, "issues": [str(exc)]}, ensure_ascii=False))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
