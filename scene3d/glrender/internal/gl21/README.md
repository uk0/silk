# SceneView 的精简 GL ABI

这是私有实现，不是通用 OpenGL SDK。维护 `generate.go` 的 58 个函数声明，然后执行：

```sh
go generate ./scene3d/glrender/internal/gl21
```

`bindings.go`、`bindings.h`、`load.inc` 均由同一声明生成，不应手工修改。
`constants.go` 是所用 Desktop GL 2.1 / EXT framebuffer 枚举值。没有新增第三方运行依赖。

- 每个 `API` 单独分配 C 函数表并持有原生库引用；失败释放部分构造，关闭释放表/引用。
- WGL 函数按当前 context 查询，排除 1/2/3/-1 无效地址，核心函数回退系统 DLL exports。
  Windows 用 `LoadLibraryExW(..., LOAD_LIBRARY_SEARCH_SYSTEM32)`，WGL 的 Go 入口也限制为系统 DLL。
- macOS 使用 OpenGL framework / CGL；Linux 使用 `libGL.so.1` / GLX。当前不实现 EGL。
- OS 线程/context 身份在构造时捕获。API 的线程字段保持不变；Renderer 在锁定本次调用
  的 OS 线程后先校验线程，再读自己的可变状态、检查 current context、进入 GL。
- 字符串/切片只在同步 cgo 调用内借用；不将 Go 指针保存到 C 表。offset 以 `uintptr_t`
  传入 C 后转换，避免在 Go 中伪造指针。调用约定包括 Win32 `__stdcall`，GL 基本类型
  宽度有 C 静态断言。函数指针加载使用 `memcpy`，不经 `void **` 破坏别名规则。
- 不创建/迁移/销毁 native context，不增加渲染线程、动画循环或 UI 子窗口。
- 默认单测覆盖无 current context 时反复打开失败、零值 API；真实驱动探针覆盖颜色/ID、
  深度、两 context 并行、错误 context/线程及并发 owner Close。只编译不代表驱动成功。

此替换不实现零读回、MSAA、PBR 或硬件实例化；不要据此宣称整个 3D 引擎目标完成。

ABI 依据：[OpenGL Registry](https://registry.khronos.org/OpenGL/index_gl.php)、
[WGL GetProcAddress](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-wglgetprocaddress)、
[GLX 1.4](https://registry.khronos.org/OpenGL/specs/gl/glx1.4.pdf)。
