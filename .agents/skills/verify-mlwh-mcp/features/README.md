# mlwh-mcp-server Feature Map

| Feature                                           | Entry points                                   | Helper                                                 |
| ------------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------ |
| [Agent feedback tool](feedback-tool.md)           | `mlwh_provide_feedback` over stdio and HTTP    | `drive-feedback.sh`, `drive-feedback-disabled.sh`      |
| [Tools and resources](tools-and-resources.md)     | `initialize`, `tools/list`, `resources/*`      | `drive-basics.sh`                                      |
| [Streamable HTTP transport](http-transport.md)    | `--http` / `MLWH_HTTP_ADDR`, `/mcp`, `/health` | `drive-basics.sh`, `drive-feedback.sh`                 |
| [Read tools](read-tools.md)                       | `mlwh_freshness`, `mlwh_classify_identifier`   | `drive-basics.sh`, `mcpclient call`                    |

Every feature assumes a stack from the skill's Launch step and a passing
Doctor. Scripts live in `../scripts/`.
