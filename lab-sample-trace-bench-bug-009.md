# BUG-009

## bug_id
lab-sample-trace-bench-bug-009

## task_type
bugfix

## bug_category
concurrency

## repro_determinism
deterministic

## repo_url
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug009_green

## green_test_branch
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug009_red

## baseline_commit
1e97f84ce5ff67ae88a36c49c4fd718c16d1d8ca

## test_commit
828a82a539e51907915f2b877eaa1fd5bfeb0d70

## fix_commit
6273160754495a331976bcfce6e1674b29cfc9fb

## go_version
go1.26.1 windows/amd64

## user_query
容量预警把恰好达到阈值的区域排除，造成满载边界没有告警。

## verify_cmds
```powershell
go test -race -count=10 ./internal/service -run TestBug009
```
## gold_root_cause
中文根因：并发告警评估在阈值边界读取利用率后错误跳过相等值，导致告警状态更新链路没有写入 pause_gate_in；证据是 BUG-009 红测在 80% 容量阈值下得到 0 条告警。
生产文件/符号：internal/service/policy.go / CapacityAlerts
调用链：ZoneUtilization → CapacityAlerts → gate-in/rehandle decision
失效原因：缺陷改变了生产逻辑的边界或状态约束，使合法输入得到错误结果。
证据：红测提交 828a82a539e51907915f2b877eaa1fd5bfeb0d70 在 bug009_red 失败，修复提交 6273160754495a331976bcfce6e1674b29cfc9fb 在 bug009_fix 通过。

## success_criteria
目标行为：利用率达到或超过阈值即告警；边界：恰好阈值与 95% 紧急阈值；合法场景：低于阈值不告警；验证标准：等于阈值产生 pause_gate_in，达到 95% 产生 urgent_rehandle。

## validation
red_failed=True
fix_passed=True
trajectory_url=未生成（本地模型轨迹采集工具不可用）
