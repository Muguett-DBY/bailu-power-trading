# 收录的上游技能 / Skills taken from upstream

这些技能原样复制自上游，连同各自的许可证文件；没有改动。

| 技能 | 来源 | 提交 | 许可证 |
| --- | --- | --- | --- |
| `office/docx` | [anthropics/skills](https://github.com/anthropics/skills) · `skills/docx` | `e4bb5d960d87126ff3b3a8af1a104b7db67560ba` | Apache-2.0（`LICENSE.txt`） |
| `office/xlsx` | anthropics/skills · `skills/xlsx` | `e4bb5d9` | Apache-2.0 |
| `office/pptx` | anthropics/skills · `skills/pptx` | `e4bb5d9` | Apache-2.0 |
| `office/pdf` | anthropics/skills · `skills/pdf` | `e4bb5d9` | Apache-2.0 |
| `design/canvas-design` | anthropics/skills · `skills/canvas-design` | `e4bb5d9` | Apache-2.0 |
| `design/brand-guidelines` | anthropics/skills · `skills/brand-guidelines` | `e4bb5d9` | Apache-2.0 |
| `design/theme-factory` | anthropics/skills · `skills/theme-factory` | `e4bb5d9` | Apache-2.0 |
| `design/frontend-design` | anthropics/skills · `skills/frontend-design` | `e4bb5d9` | Apache-2.0 |
| `design/web-design-guidelines` | [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills) · `skills/web-design-guidelines` | `5a37a6329fbad3666c3207392a6a7c0110e92efb` | MIT（上游 README 所述） |

上游 README 在该提交处写明文档技能（docx、pdf、pptx、xlsx）同样是 Apache 2.0 开源的。

这些技能运行时可能用到 Python 库（如 python-docx、openpyxl、python-pptx、pdfplumber、reportlab）和命令行工具（如 LibreOffice、poppler 的 pdftotext、qpdf）。它们不随本框架分发，按各自 `SKILL.md` 里的说明在需要时安装。

The skills above are copied unchanged from their upstream repositories at the commits named, each with its own license file. The Python libraries and command-line tools they call are not part of this framework; see each `SKILL.md`.
