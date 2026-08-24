# BUG-007

## bug_id
lab-sample-trace-bench-bug-007

## task_type
bugfix

## bug_category
slice

## repro_determinism
deterministic

## repo_url
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug007_green

## green_test_branch
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug007_red

## baseline_commit
afeac4bb28f77132a45cd3695e183d232233e7b3

## test_commit
632b0fe92cad53b15c5f63a0e89edc637c92447b

## fix_commit
fb4808927d55ad6d4ff43b8e35af07f2e6c323f7

## go_version
go1.26.1 windows/amd64

## user_query
班次边界把 08:00 错分到 night，导致报表和调度使用错误的班次窗口。

## verify_cmds
```powershell
go test -count=1 ./internal/service -run TestBug007
```
## gold_root_cause
中文根因：小时边界使用了小于等于 8，08:00 仍落入夜班；证据是 BUG-007 红测得到 night。
生产文件/符号：internal/service/policy.go / CurrentShift
调用链：CurrentShift → ShiftSummary → MoveSummary
失效原因：缺陷改变了生产逻辑的边界或状态约束，使合法输入得到错误结果。
证据：红测提交 632b0fe92cad53b15c5f63a0e89edc637c92447b 在 bug007_red 失败，修复提交 fb4808927d55ad6d4ff43b8e35af07f2e6c323f7 在 bug007_fix 通过。

## success_criteria
目标行为：08:00 起进入 morning；边界：00:00、08:00、16:00；合法场景：全天任意时间；验证标准：三个班次边界均落入正确窗口。

## validation
red_failed=True
fix_passed=True
trajectory_url=未生成（本地模型轨迹采集工具不可用）
