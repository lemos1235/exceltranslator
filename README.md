# Excel Translator

An easy-to-use tool for translating Excel, Word, and PowerPoint files.

## Key Features

- Supports **XLSX**, **DOCX**, and **PPTX**.
- Translates text in Excel cells/shapes, Word documents, and PowerPoint slides (including shapes and tables).
- Preserves original formatting and styles.
- Utilizes advanced AI models for high-quality translation.
- Provides a clean and intuitive graphical user interface (GUI).

## Configuration

Upon its first run, the application creates a default configuration file in the user's configuration directory:

- Windows: `%APPDATA%\Excel-Translator\config.toml`
- macOS: `~/Library/Application\ Support/Excel-Translator/config.toml`

You can edit this configuration file to customize the application's behavior:

```toml
[llm]
base_url = 'https://apis.iflow.cn/v1/chat/completions'
api_key = 'sk-'
model = 'iflow-rome-30ba3b'
prompt = '翻译为简体中文。保留所有数字和字母。若原样为中文则不处理。仅输出译文，禁止回复译文以外的任何内容。'

[translation]
# Translate only CJK (Chinese, Japanese, Korean) text
cjk_only = false
# Max concurrent LLM translation requests
max_concurrent_requests = 4
```

## GUI

To install dependencies, please refer to https://github.com/mappu/miqt

To compile and package:

```bash
chmod +x ./build_macapp.sh
./build_macapp.sh
```

Screenshot:

![screenshot.png](demo/screenshot.png)
