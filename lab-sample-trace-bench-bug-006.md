# BUG-006

## bug_id
lab-sample-trace-bench-bug-006

## task_type
diagnosis

## bug_category
context

## repro_determinism
deterministic

## repo_url
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug006_green

## green_test_branch
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug006_red

## baseline_commit
5f0b902c77c8156df4c9ea32d4512430732ce742

## test_commit
c9914d637674cb4986dc9bd98930f65086062ed5

## fix_commit
168f271c99ae580287b35dd6a63141d39f185f17

## go_version
go1.26.1 windows/amd64

## user_query
恰好剩余 30 分钟的任务截止时间被分到 soon，而不是 critical。

## verify_cmds
```powershell
go test -count=1 ./internal/service -run TestBug006
```
## gold_root_cause
中文根因：临界比较使用了严格小于，排除了恰好 30 分钟的边界；证据是 BUG-006 红测得到 soon。
生产文件/符号：internal/service/operational_validation.go / ClassifyDeadline
调用链：CreateTask → Weight/ClassifyDeadline → dispatcher priority
失效原因：缺陷改变了生产逻辑的边界或状态约束，使合法输入得到错误结果。
证据：红测提交 c9914d637674cb4986dc9bd98930f65086062ed5 在 bug006_red 失败，修复提交 168f271c99ae580287b35dd6a63141d39f185f17 在 bug006_fix 通过。

## success_criteria
目标行为：剩余时间小于或等于 30 分钟为 critical；边界：正好 30 分钟；合法场景：expired、critical、soon、normal 四类；验证标准：各边界分类与策略一致。

## validation
red_failed=True
fix_passed=True
trajectory_url=未生成（本地模型轨迹采集工具不可用）
