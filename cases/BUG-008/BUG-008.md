# BUG-008

## bug_id
lab-sample-trace-bench-bug-008

## task_type
diagnosis

## bug_category
error

## repro_determinism
deterministic

## repo_url
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug008_green

## green_test_branch
https://github.com/zhangkui/lab-sample-trace-bench/tree/bug008_red

## baseline_commit
7c6c25a95c401ec28b9adc43e14cecfb406cc120

## test_commit
7521ef938cf40423f5e3105d960a2fa1d0d16911

## fix_commit
65fed87a7f590cd7d9b43dbac3639d1aab247171

## go_version
go1.26.1 windows/amd64

## user_query
装船任务缺少 vessel call 引用时被错误准入，后续无法绑定船期截止时间。

## verify_cmds
```powershell
go test -count=1 ./internal/service -run TestBug008
```
## gold_root_cause
中文根因：船期引用为空的准入状态机在资源引用缺失时继续创建任务，导致后续船期截止时间解析链路无法建立并放行无效任务；证据是 BUG-008 红测显示 load task without vessel call was admitted。
生产文件/符号：internal/service/policy.go / CheckTaskAdmission
调用链：CreateTask → CheckTaskAdmission → vessel cut-off validation
失效原因：缺陷改变了生产逻辑的边界或状态约束，使合法输入得到错误结果。
证据：红测提交 7521ef938cf40423f5e3105d960a2fa1d0d16911 在 bug008_red 失败，修复提交 65fed87a7f590cd7d9b43dbac3639d1aab247171 在 bug008_fix 通过。

## success_criteria
目标行为：load/discharge 必须绑定 vessel call；边界：restow、inspection 可无船期；合法场景：有效 vessel/voyage 引用；验证标准：缺失引用拒绝，有效引用继续后续校验。

## validation
red_failed=True
fix_passed=True
trajectory_url=未生成（本地模型轨迹采集工具不可用）
