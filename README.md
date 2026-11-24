# dkit

> **一个能管道的 DevToys。** 31 个命令，一个静态二进制，`stdin → stdout` 串起来用。

[![release](https://img.shields.io/github/v/release/fansa92/dkit?include_prereleases&sort=semver)](https://github.com/fansa92/dkit/releases)
[![ci](https://github.com/fansa92/dkit/actions/workflows/ci.yml/badge.svg)](https://github.com/fansa92/dkit/actions/workflows/ci.yml)
[![go](https://img.shields.io/github/go-mod/go-version/fansa92/dkit)](go.mod)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

每天都会碰到的那点小事：粘一个 JWT 想看看里面是谁、`1700000000` 是几号、这个证书什么时候过期、两个 JSON 差在哪、`/26` 网段有多少可用地址。为这些去开一个 GUI 工具箱太重，`dkit` 把它们做成一条命令。

```console
$ echo "$TOKEN" | dkit jwt --payload | dkit jsonpath --raw '$..sub'
1234567890
```

## 安装

**下载二进制**（推荐）—— 到 [Releases](https://github.com/fansa92/dkit/releases) 挑一个包，解压即用，无需运行时：

```
dkit_0.1.1_windows_amd64.zip
dkit_0.1.1_linux_amd64.tar.gz
dkit_0.1.1_darwin_arm64.tar.gz
```

共 17 个目标，覆盖三大桌面系统以及主流 Unix：

| 系统 | 架构 |
|---|---|
| Windows | amd64、arm64 |
| macOS | amd64、arm64 |
| Linux | amd64、arm64、armv7、riscv64、loong64 |
| FreeBSD | amd64、arm64 |
| OpenBSD | amd64、arm64 |
| NetBSD | amd64、arm64 |
| Solaris / illumos | amd64 |

全部为 `CGO_ENABLED=0` 的纯静态二进制，不依赖 libc 或 Go 运行时。

**用 Go 装**：

```bash
go install github.com/fansa92/dkit@latest   # 需要 Go 1.26+
```

**从源码构建**：

```bash
git clone https://github.com/fansa92/dkit
cd dkit
go build -trimpath -ldflags "-s -w" -o dkit .
```

## 上手

```bash
dkit about                              # 这是什么、这个二进制是哪次构建
dkit help                               # 命令总表（带别名）
dkit json fmt api.json                  # 格式化，-i 4 改缩进
dkit yaml to-json config.yaml | dkit json min
dkit jwt eyJhbGciOi...                  # 解 claims，标出 exp 是否过期
dkit ts 1700000000                      # 时间戳 → 本地/UTC/RFC3339/周几
dkit ts -1h                             # 相对时间也认
dkit cron "0 9 * * 1-5"                 # 人话解释 + 接下来 5 次触发
dkit cert cert.pem                      # 主体、签发者、SAN、指纹、剩余天数
dkit ip 10.0.0.0/22 -n 3                # 掩码、广播、可用主机、前几个地址
dkit hash ./app.bin                     # 一次给出 md5/sha256/blake2b/crc32
dkit diff old.txt new.txt               # 统一 diff，带颜色
dkit img svg icon.svg -w 512 -o icon.png
```

管道是主要用法：

```bash
# 批量格式化，只列出会被改的文件
find . -name '*.json' -exec sh -c 'dkit json check "$1" || echo "$1"' _ {} \;

# CI 里卡过期 token
dkit jwt "$TOKEN" >/dev/null || exit 1

# 日志里挑 IP 再算网段
dkit regex -p '\b\d+\.\d+\.\d+\.\d+\b' access.log | sort -u
```

## 命令

每个命令都有 1–2 字母的别名（`dkit j` = `dkit json`），`dkit list -a` 看全表。

### 数据与格式

| 命令 | 别名 | 说明 |
|---|---|---|
| `json` | `j` | 格式化 / 压缩 / 校验 / 转表格 / 转 CSV / 生成 C# 或 PHP / JSON Schema 校验 |
| `jsonpath` | `jp` | `$.users[*].name`、`$..id` 查询，`--raw` 输出裸值 |
| `yaml` | `y` | YAML ↔ JSON |
| `toml` | `tm` | TOML ↔ JSON |
| `xml` | `x` | 格式化、校验、XML ↔ JSON |
| `sql` | `s` | 基础重排：子句换行、关键字大写（不做语义解析） |
| `diff` | `d` | 两文件统一 diff，`-c` 改上下文行数 |

### 编码与解码

| 命令 | 别名 | 说明 |
|---|---|---|
| `enc` | `e` | base64 / base64 图片 / url / html / hex / base58 / gzip / zlib / quoted-printable / punycode / 转义 |
| `jwt` | `jw` | 解 header + payload，`exp` 过期即退出码 1，`--payload` 只吐 JSON |
| `cert` | `ce` | 解析 PEM 证书链 |
| `qr` | `q` | 生成二维码 PNG，或 `--terminal` 直接打在终端 |
| `base` | `b` | 任意进制互转，支持 `0xff` 前缀和大数 |

### 生成与校验

| 命令 | 别名 | 说明 |
|---|---|---|
| `hash` | `h` | md5 / sha1 / sha224 / sha256 / sha384 / sha512 / sha3-256 / sha3-512 / blake2b-256 / blake2b-512 / crc32 / adler32，`-c` 校验摘要 |
| `gen` | `g` | UUID v4/v7、ULID、密码、Lorem、RSA 密钥、TOTP、bcrypt、随机字节 |
| `regex` | `r` | 匹配列表、替换、`-i` 忽略大小写 |
| `semver` | `sv` | 解析、比较、bump major/minor/patch |
| `iban` | `ib` | IBAN 校验并拆出国家码、银行码、账号 |

### 时间与调度

| 命令 | 别名 | 说明 |
|---|---|---|
| `ts` | `t` | 时间戳 / 日期 / 相对时间三向转换，`--tz` 指定时区，`--json` 给脚本 |
| `cron` | `cr` | 解析 5/6 段表达式与 `@daily` 宏，解释含义并列出下次触发 |

### 网络与文件

| 命令 | 别名 | 说明 |
|---|---|---|
| `ip` | `i` | IPv4/IPv6 地址或网段：掩码、广播、可用主机数、地址列举 |
| `urlq` | `u` | URL 或 query string 拆解 |
| `curl` | `c` | curl 命令转 Go / Python / JavaScript 代码 |
| `file` | `f` | 大文件分片与合并、重复文件检测（sha256 并发）、文件信息 |

### 图形与文本

| 命令 | 别名 | 说明 |
|---|---|---|
| `img` | `im` | 信息 / 格式转换 / 缩放 / 裁剪 / 重新压缩 / 色盲模拟 / SVG → PNG |
| `color` | `co` | hex、rgb、hsl、hsv 互转，WCAG 对比度检查 |
| `md` | `m` | Markdown 渲染为 HTML |
| `css` `html` | `cs` `ht` | 重新缩进 |
| `text` | `tx` | 大小写与命名转换（camel/snake/kebab/constant）、计数、统计、排序、去重、分隔符表格、两列对比 |

### 其他

| 命令 | 别名 | 说明 |
|---|---|---|
| `chmod` | `cm` | 八进制 ↔ 符号权限 |
| `about` | `a` | 一句话简介、构建信息、仓库链接 |

## 约定

**输入三态。** 没有参数或第一个参数是 `-` → 读 stdin；第一个参数是存在的文件 → 读该文件；否则当作字面量字符串。所以 `dkit hash notes.txt` 和 `dkit hash --literal hello` 都成立，`--literal` 用于文件名恰好和字面量重名时强制走字面量。

**输出。** 默认 stdout，`-o file` 写文件，`-o -` 回 stdout。`--json` 只出现在查询型命令上（`jwt` `cert` `ts` `cron` `base` `jsonpath`），转换型命令的输出本身就是数据。

**退出码。**

| 码 | 含义 |
|---|---|
| `0` | 成功 |
| `1` | 执行失败：token 过期、jsonpath 无匹配、摘要不一致 |
| `2` | 用法错误：未知命令、未知 flag、参数不合法 |

这样 `set -e` 能停，脚本也能区分「查了但没有」和「你用错了」。

**颜色。** 自动检测 TTY；`NO_COLOR=1` 或全局 `--no-color` 关闭。进管道不会污染数据。

**帮助。** `dkit help <cmd>` 和 `dkit <cmd> --help` 输出同一份内容，包括 flag 表。子命令也能看：`dkit json fmt --help`。

## 构建与发布

本地交叉编译（`CGO_ENABLED=0`，纯静态）：

```bash
for t in \
  windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 \
  linux/amd64 linux/arm64 linux/arm/7 linux/riscv64 linux/loong64 \
  freebsd/amd64 freebsd/arm64 openbsd/amd64 openbsd/arm64 \
  netbsd/amd64 netbsd/arm64 solaris/amd64 illumos/amd64
do
  os=${t%%/*}; rest=${t#*/}; arch=${rest%%/*}; arm=""
  case "$t" in */*/*) arm=${rest#*/} ;; esac
  ext=""; [ "$os" = windows ] && ext=.exe
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=$arm go build -trimpath -ldflags "-s -w" \
    -o "dist/dkit_${os}_${arch}${arm:+v$arm}${ext}" .
done
```

产物 8–9 MB。构建信息在发布时注入，`dkit about` 会显示对应的 commit 与构建时间：

```bash
go build -ldflags "-s -w \
  -X main.version=0.1.1 \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
```

GitHub Actions 两套：

- **`.github/workflows/ci.yml`** — push / PR 时跑 `gofmt` 检查、`go vet`、以及 17 个目标平台的交叉编译，防止某平台编译不过。
- **`.github/workflows/release.yml`** — 推 `v*` tag（或手动触发）时，17 个目标并行编译，打成 `.zip` / `.tar.gz`，附 README 与 LICENSE，汇总 `checksums.txt` 并创建 Release。

发布新版本：

```bash
git tag v0.1.1 && git push origin v0.1.1
```

## 与 DevToys 的关系

灵感和功能清单来自 [DevToys](https://github.com/DevToys-app/DevToys)（C# / WinUI 的图形化开发者工具箱）。`dkit` 是同一批工具的 CLI 重写：覆盖其 63 项功能中的 61 项，砍掉了纯 Go 无法可靠实现的 XSD 校验与生成，其余保持单二进制、零运行时。

有 5 处是**降级实现**，别当等价物用：

| 命令 | 边界 |
|---|---|
| `sql` | 词级重排，不解析语义，复杂子查询缩进一般 |
| `img compress` | 仅用最高压缩率重编码，没有色彩量化，不如 pngquant |
| `img svg` | 不支持圆角矩形与部分滤镜，渐变、路径、透明度正常 |
| `json schema` | draft-07 常用关键字子集，`$ref` 只识别不解析 |
| `md` | 输出 HTML 文件，不是 GUI 那种边打字边预览 |

## License

[MIT](LICENSE)
