---
name: submit-pr
description: 当需要把本仓库（OpsMind）的改动提交为 PR 时使用。定义了 scutmmq 的 fork 工作流、分支命名、提交署名、PR 目标与「提交前必须人工确认」等硬性规则。涉及 git commit / git push / 开 PR 时务必先读本 skill。
---

# OpsMind 提交 PR 流程（scutmmq fork 工作流）

本仓库是小组作业。我（scutmmq）对组长仓库只有**只读**权限，所有贡献走
**fork + Pull Request**。本 skill 记录固定流程，任何模型按此执行即可。

## 0. 仓库角色（git remote）

| remote | 指向 | 用途 |
|---|---|---|
| `fork` | `github.com/scutmmq/OpsMind` | **我的** fork，所有 push 都推到这里 |
| `origin` | `github.com/int2t05/OpsMind` | **组长**的主仓库，只 PR、**绝不 push** |

> ⚠️ 注意 `origin` 是组长仓库（只读）。push 必须显式写 `fork`，
> 例如 `git push -u fork <branch>`，绝不能 `git push origin`。

## 1. 铁律（违反任何一条都算错）

1. **绝不自动 push、绝不自动开 PR。** 每次 push / `gh pr create` 之前，
   必须把改动和计划讲清楚，得到用户**明确确认**后才执行。
2. **提交署名只能是 scutmmq，绝不带 Claude / AI 署名。**
   - author 与 committer 均为 `scutmmq <2018147749@qq.com>`
   - commit message **不得**包含 `Co-Authored-By: Claude ...` 或任何 AI 标记
   - 全局已配置 `user.name=scutmmq` / `user.email=2018147749@qq.com`，
     提交前用 `git log -1 --format='%an <%ae>'` 复核。
3. **中文 commit message，格式 `类型: 简短描述`**
   （如 `fix: 修复多轮对话消息重叠错位`、`feat: 实现 BM25 混合检索`）。
4. **一个 PR 只做一件事。** 一个 bug / 一个功能一个分支一个 PR，
   不把无关改动混在一起。
5. **PR 前先自测验证。** 前端用 Playwright 实测、后端用 curl / 数据库核对，
   在 PR 描述里写清楚验证方式与结果。
6. **不提交本地环境产物**：`.env`（含 DeepSeek key）、`docker-compose.override.yml`、
   截图 png、`docs/*-plan.md` 等本地文件不进提交；Dockerfile 的本地 arm64 临时改动
   要还原成 `amd64` 再提交。

## 2. 标准流程

```bash
# (1) 同步组长最新代码到本地 main
git checkout main
git fetch origin
git merge --ff-only origin/main          # 或 git reset --hard origin/main

# (2) 从最新 main 切出功能分支（命名：fix/xxx 或 feat/xxx）
git checkout -b fix/<简短英文描述>

# (3) 改代码 → 自测验证（见铁律 5）

# (4) 提交（确认署名！）
git add <改动文件>                        # 精确 add，不要 git add .
git commit -m "fix: <中文描述>"
git log -1 --format='%an <%ae>%n%s'      # 复核：scutmmq + 无 AI 署名

# (5) —— 到此停下，向用户汇报，等待确认 ——

# (6) 用户确认后，push 到我的 fork
git push -u fork fix/<简短英文描述>

# (7) 用户确认后，开 PR 到组长仓库的 main
gh pr create --repo int2t05/OpsMind \
  --base main --head scutmmq:fix/<简短英文描述> \
  --title "fix: <中文描述>" \
  --body "<问题 / 改动 / 验证方式>"
```

## 3. 如果署名错了（带上了 Claude / 不是 scutmmq）

```bash
# 改最近一次提交的 message + author（去掉 AI 署名）
git commit --amend --author="scutmmq <2018147749@qq.com>" -m "fix: <中文描述>"
# 若已 push，需要 force（仅对自己 fork 的分支，确认后再做）
git push -f fork <branch>
```
如果 PR 已经开了且署名有问题：关掉该 PR、删分支、按上面修好后重开。

## 4. 不是所有改动都要 PR

- **代码改动** → 走 PR。
- **纯数据修复 / 运维操作**（例如重新发布文章补回向量、改数据库数据）
  **不提 PR**，因为没有仓库代码变化。这类操作只在本地/各自环境执行，
  必要时在群里说明，让组员各自处理。
