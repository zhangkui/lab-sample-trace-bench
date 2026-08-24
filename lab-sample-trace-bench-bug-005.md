# BUG-005

## bug_id
lab-sample-trace-bench-bug-005

## task_type
bugfix

## bug_category
concurrency

## repro_determinism
deterministic

## repo_url
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug005_green

## green_test_branch
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug005_red

## baseline_commit
506c902d7590989f8b7b5fd7f29c89ba31ca9a2f

## test_commit
b7caae4d5a91a299de6cc333ba4416bc8a5f1383

## fix_commit
edb204625c6b6afe9afbe4d71c0783760e65b7fc

## go_version
go1.26.1 windows/amd64

## user_query
船期状态机允许已离港船舶重新进入 alongside，形成并发调度中的非法活动窗口。

## verify_cmds
```powershell
go test -race -count=10 ./internal/service -run TestBug005
```
## gold_root_cause
中文根因：状态机为 departed 增加了 alongside 转移；证据是 BUG-005 红测直接观察到 departed→alongside 返回 true。
生产文件/符号：internal/service/schedule.go / validScheduleTransition
调用链：AdvanceCall → validScheduleTransition → task admission/dispatcher
失效原因：缺陷改变了生产逻辑的边界或状态约束，使合法输入得到错误结果。
证据：红测提交 b7caae4d5a91a299de6cc333ba4416bc8a5f1383 在 bug005_red 失败，修复提交 edb204625c6b6afe9afbe4d71c0783760e65b7fc 在 bug005_fix 通过。

## success_criteria
目标行为：departed 为终态；边界：所有终态转移均拒绝；合法场景：proposed→arriving→alongside→departed；验证标准：非法转移返回 false，合法顺序保持通过。

## validation
red_failed=True
fix_passed=True
trajectory_url=未生成（本地模型轨迹采集工具不可用）
