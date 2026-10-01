# adsched — 直投广告保量排期库

纯内存的 Go 库，由上层投放任务直接调用，不含接口层、不依赖任何外部服务。
根据流量预测为剩余时段生成排期计划，在尽量履约的前提下贴近约定投放节奏。

## 模型

- 一天固定 48 个 30 分钟时段，当前时点 `Input.Now` 总在时段边界。
- 每个广告位属于一个发布方并固定一个素材尺寸，`Forecast[t]` 给出各时段预测容量。
- 每份合同包含发布方、可投放尺寸集合、整数保证量 `Q`、当天起止时段 `[Start, End)`
  （含起点、不含终点）以及均匀（`Even`）或前置（`FrontLoaded`）节奏，
  `Actual` 为截至当前边界的实际累计投放数（不超过 `Q`）。
- 计划给出每份合同在各广告位、各剩余时段的整数曝光数，满足：
  发布方与尺寸匹配、时段在合同范围内、广告位各时段总分配不超过预测容量、
  合同实际累计加未来分配不超过 `Q`。

## 优化目标（严格先后）

1. 最小化所有合同最终未完成保证量之和；
2. 在该最优值下，最小化总节奏偏差 —— 每份合同尚未结束的履约时段，
   在各时段末「预计累计投放量（含此前实际投放）与累计目标之差的绝对值」之和。

累计目标始终使用合同原始保证量与原始起止时段（`H = End - Start`，
`k` 为从合同起点起已结束的时段数）：

- 均匀：`floor(Q*k/H)`
- 前置：`floor(Q*k*(2H-k)/H²)`

实现上将问题建模为最小费用最大流：最大流对应最小总欠量，
在最大流中费用最小对应总节奏偏差最小，因此一次求解即满足字典序目标。
即使容量不足，也返回全局最优的欠量计划，不会拒绝排期。
用更新后的预测容量与实际累计量重新调用 `Schedule` 即可中途重排，过去投放不可改写。

## 使用

```go
in := adsched.Input{
    Now: 2, // 当前时段边界
    Slots: []adsched.Slot{
        {ID: "slot-1", Publisher: "pub-a", Size: "300x250", Forecast: fc}, // fc 长度 48
    },
    Contracts: []adsched.Contract{
        {ID: "c-1", Publisher: "pub-a", Sizes: []string{"300x250"},
         Q: 5, Start: 0, End: 4, Pacing: adsched.Even, Actual: 0},
    },
}
plan, err := adsched.Schedule(in)
if err != nil { /* 输入非法（如 Forecast 长度不为 48） */ }
for _, a := range plan.Allocations {
    // a.ContractID / a.SlotID / a.Period / a.Amount
}
fmt.Println(plan.Delivered("c-1"))               // 未来分配总量
fmt.Println(plan.Underdelivery(in.Contracts[0])) // 最终欠量
```

## 测试

```sh
go test ./...
```

覆盖场景：尺寸灵活/受限合同争用容量（无可避免欠量）、中途重排
（Q=5、H=4、前两段实际为 0 时均匀依次分 3/2、前置依次分 4/1）、
容量不足时的最优欠量、欠量目标优先于节奏偏差、
发布方/尺寸/时段范围/容量/保证量上限约束、已结束合同与非法输入。
