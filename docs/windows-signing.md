# Windows 发布签名

公开的 Windows 可执行文件在创建 ZIP 和 SHA-256 清单之前完成 Authenticode 签名。签名需要受信任发布者的身份和时间戳。GitHub Release 提供文件托管，校验和提供文件完整性验证。Windows 执行信任适用其独立的应用控制策略。[Microsoft Smart App Control 签名文档](https://learn.microsoft.com/en-us/windows/apps/develop/smart-app-control/code-signing-for-smart-app-control)

## 用 GitHub Actions 签名

1. 配置经身份核验的 Azure Artifact Signing 账号和 Public Trust 证书配置，给 GitHub OIDC 身份分配 `Artifact Signing Certificate Profile Signer`。按仓库的 `windows-signing` 环境限制可触发的分支。所在地区和身份类型按微软当前服务要求核对。[Artifact Signing 快速入门](https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart) · [Public Trust](https://learn.microsoft.com/en-us/azure/artifact-signing/concept-trust-models) · [官方 GitHub Action](https://github.com/Azure/artifact-signing-action)
2. 在该环境中配置 `AZURE_CLIENT_ID`、`AZURE_TENANT_ID` 和 `AZURE_SUBSCRIPTION_ID`，以及 `BAILU_SIGNING_ENDPOINT`、`BAILU_SIGNING_ACCOUNT` 和 `BAILU_SIGNING_PROFILE` 三个变量。
3. 先提交新版本源码，确保 Go 工具与 `scripts/deploy.py` 的版本一致。在 `main` 上手动运行 **Sign Windows release tools**。下载此次运行的 `bailu-signed-windows-<提交 SHA>`。Windows x64 和 Arm64 各有一份签名文件。

也可以使用受信任 CA 的代码签名证书或获准使用的开源签名服务。构建时从准备发布的干净提交生成两个 Windows 目标，保留 Go VCS 构建信息，并使用嵌入的 Authenticode 签名及有效时间戳。文件按工作流的目录结构交给发布器。[Microsoft 的公开签名选项](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options) · [SignPath Foundation](https://signpath.org/)

## 发布与验证

在 Windows 发布机切到签名时的提交。解压签名产物，目录应包含 `windows/amd64/bailu.exe` 和 `windows/arm64/bailu.exe`。保留两份已签名文件作为此次发布的输入，重试时沿用它们。

```powershell
uv run python scripts/publish.py vX.Y.Z --signed-windows-dir "<已解压的签名产物目录>"
```

发布器检查 Windows 签名和时间戳，以及 Go 模块、目标架构和源码提交信息，再生成 ZIP 和校验清单。签名版本通过两边的正式发布及版本化 CDN 标签分发。发布后在保持应用控制启用的 Windows 环境安装、升级并执行自检，确认目标设备的策略允许该发布者。既有版本的文件、校验清单和标签保留原有内容。
