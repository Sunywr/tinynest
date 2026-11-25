// Command tinynest-admin performs the operator-side database operations that
// must never be reachable from the public HTTP API:
//
//	baseline-capture  record identity row counts and the journal balance
//	purge             delete every business row, keeping users and permissions
//	verify            prove the purge result and that identity data survived
//	identity-export   write the identity tables as JSON lines
//	identity-import   load an identity export into a fresh V2 baseline
//
// The binary reuses exactly the same environment and credential files as the
// API server, so it can never point at a different database by accident.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"tinynest/api/internal/dbadmin"
	"tinynest/api/internal/platform/config"
)

const usage = `tinynest-admin - TinyNest 运维命令（危险操作需显式确认）

用法:
  tinynest-admin [-driver sqlite|mysql] <command> [flags]

命令:
  baseline-capture   -out=<path>                       采集清理前的身份计数、迁移版本与借贷合计
  purge              [-dry-run]                        只打印将删除的表与行数
  purge              -execute -confirm=<db> -yes-service-stopped [flags]   执行清理
  verify             [-baseline=<path>]                只读校验：业务表全空且身份数据未变
  identity-export    -out=<path>                       导出身份表为 JSON 行
  identity-import    -in=<path> [-dry-run]             把身份数据导入空的 V2 基线库
  seed-categories    [-household=<id>]                 重播默认分类并补齐图标与颜色

purge 额外开关:
  -household=<id>        多家庭库必须显式给出以确认执行；注意删除不按家庭过滤，
                         所有家庭的业务行都会被清空（该 ID 只决定重播种哪个家庭）
  -keep-audit            保留 audit_logs 与 schedule_runs
  -reseed-categories     清理后重新播种默认分类（默认关闭）
  -allow-unbalanced      分录已不平衡时仍继续（默认拒绝）

verify 额外开关:
  -allow-seeded-categories  允许重新播种的默认分类行

连接参数来自环境变量，与 API 服务完全一致：
  TINYNEST_DB_DRIVER, TINYNEST_DATA_DIR, TINYNEST_DB_PATH,
  TINYNEST_MYSQL_SOCKET, TINYNEST_MYSQL_DATABASE, TINYNEST_MYSQL_USER,
  TINYNEST_MYSQL_PASSWORD_FILE (或 CREDENTIALS_DIRECTORY/mysql-password)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		if len(args) == 0 {
			return errors.New("缺少子命令")
		}
		return nil
	}

	driver := ""
	if args[0] == "-driver" || args[0] == "--driver" {
		if len(args) < 2 {
			return errors.New("-driver 需要 sqlite 或 mysql")
		}
		driver = args[1]
		args = args[2:]
		if len(args) == 0 {
			return errors.New("缺少子命令")
		}
	}
	if driver != "" {
		if err := os.Setenv("TINYNEST_DB_DRIVER", driver); err != nil {
			return err
		}
	}

	command, rest := args[0], args[1:]
	switch command {
	case "baseline-capture":
		return runBaselineCapture(rest)
	case "purge":
		return runPurge(rest)
	case "verify":
		return runVerify(rest)
	case "identity-export":
		return runIdentityExport(rest)
	case "identity-import":
		return runIdentityImport(rest)
	case "seed-categories":
		return runSeedCategories(rest)
	default:
		fmt.Print(usage)
		return fmt.Errorf("未知子命令 %q", command)
	}
}

func runBaselineCapture(args []string) error {
	fs := flag.NewFlagSet("baseline-capture", flag.ContinueOnError)
	out := fs.String("out", "", "输出 JSON 路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("baseline-capture 需要 -out=<path>")
	}
	cfg, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()
	ctx := context.Background()

	schema, err := admin.Tables(ctx)
	if err != nil {
		return err
	}
	snap, err := admin.SnapshotNow(ctx, schema)
	if err != nil {
		return err
	}
	snap.Database = cfg.Name()
	if err := writeJSON(*out, snap); err != nil {
		return err
	}
	printSnapshot("清理前基线", snap)
	if drift := snap.Unclassified(schema.All()); len(drift) > 0 {
		fmt.Println("⚠ 未分类表（既不在清理集合也不在身份集合，请人工确认）: " + strings.Join(drift, ", "))
	}
	fmt.Printf("已写入 %s\n", *out)
	return nil
}

func runPurge(args []string) error {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "只打印计划，不修改数据")
	execute := fs.Bool("execute", false, "实际执行删除")
	confirm := fs.String("confirm", "", "必须与数据库名完全一致")
	serviceStopped := fs.Bool("yes-service-stopped", false, "确认 API 服务已停止")
	household := fs.String("household", "", "多家庭库必须显式给出以确认执行；删除不按家庭过滤，该 ID 只决定重播种哪个家庭")
	keepAudit := fs.Bool("keep-audit", false, "保留 audit_logs 与 schedule_runs")
	reseed := fs.Bool("reseed-categories", false, "清理后重新播种默认分类（会削弱「全部业务表为空」的校验，仅在你需要继续使用旧界面时开启）")
	allowUnbalanced := fs.Bool("allow-unbalanced", false, "分录不平衡时仍继续")
	jsonOut := fs.String("json-out", "", "把结果 JSON 写入该路径作为证据")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun == *execute {
		return errors.New("purge 需要且只能指定 -dry-run 或 -execute 之一")
	}
	cfg, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()

	if *execute {
		if !*serviceStopped {
			return errors.New("执行清理前必须先停止 API 服务，并加 -yes-service-stopped 确认（否则调度器与邮件 worker 会边清边写）")
		}
		if *confirm != cfg.Name() {
			return fmt.Errorf("-confirm 必须与数据库名完全一致（%q），收到 %q", cfg.Name(), *confirm)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("目标: %s (%s)\n", cfg.Name(), cfg.Driver)
	result, err := admin.Purge(ctx, dbadmin.PurgeOptions{
		HouseholdID:      *household,
		DryRun:           *dryRun,
		ReseedCategories: *reseed,
		KeepAudit:        *keepAudit,
		AllowUnbalanced:  *allowUnbalanced,
	})
	if *jsonOut != "" {
		if writeErr := writeJSON(*jsonOut, result); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}

	printSnapshot("清理前", result.Before)
	fmt.Println()
	printCounts("将删除 / 已删除", result.Deleted)
	if len(result.MissingTables) > 0 {
		fmt.Println("当前 schema 中不存在的表（已跳过）: " + strings.Join(result.MissingTables, ", "))
	}
	if len(result.Reseeded) > 0 {
		fmt.Println("已重新播种默认分类的家庭: " + strings.Join(result.Reseeded, ", "))
	}
	for _, warning := range result.Warnings {
		fmt.Println("⚠ " + warning)
	}
	fmt.Println()
	if result.DryRun {
		fmt.Println("dry-run 结束：未修改任何数据。核对无误后用 -execute 执行。")
		return nil
	}
	printSnapshot("清理后", result.After)
	fmt.Println("✓ 清理完成并已提交：业务表全空，身份与权限数据未变。")
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "清理前 baseline JSON 路径（用于比对身份数据）")
	allowSeeded := fs.Bool("allow-seeded-categories", false, "允许清理后重新播种的默认分类行（配合 purge -reseed-categories）")
	jsonOut := fs.String("json-out", "", "把当前快照写入该路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()

	var baseline *dbadmin.Snapshot
	if *baselinePath != "" {
		raw, err := os.ReadFile(*baselinePath)
		if err != nil {
			return fmt.Errorf("读取 baseline 失败: %w", err)
		}
		var snap dbadmin.Snapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return fmt.Errorf("解析 baseline 失败: %w", err)
		}
		baseline = &snap
	}
	ctx := context.Background()
	current, err := admin.Verify(ctx, baseline, *allowSeeded)
	if *jsonOut != "" {
		current.Database = cfg.Name()
		if writeErr := writeJSON(*jsonOut, current); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	current.Database = cfg.Name()
	printSnapshot("当前状态", current)
	if baseline != nil {
		fmt.Println("✓ 校验通过：业务表全空，身份与权限数据与 baseline 一致。")
	} else {
		fmt.Println("✓ 校验通过：业务表全空（未提供 -baseline，未比对身份数据）。")
	}
	return nil
}

func runIdentityExport(args []string) error {
	fs := flag.NewFlagSet("identity-export", flag.ContinueOnError)
	out := fs.String("out", "", "输出 JSON 行文件路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("identity-export 需要 -out=<path>")
	}
	_, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()

	if err := os.MkdirAll(filepath.Dir(absOrDot(*out)), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	summary, err := admin.ExportIdentity(context.Background(), file)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Println("已导出身份数据: " + summary.String())
	fmt.Printf("共 %d 行 -> %s（权限 0600，含口令哈希，按机密保管）\n", summary.Rows, *out)
	return nil
}

func runSeedCategories(args []string) error {
	fs := flag.NewFlagSet("seed-categories", flag.ContinueOnError)
	household := fs.String("household", "", "只处理指定家庭")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()

	households, err := admin.SeedCategories(context.Background(), *household)
	if err != nil {
		return err
	}
	fmt.Printf("已重播默认分类的家庭: %s\n", strings.Join(households, ", "))
	return nil
}

func runIdentityImport(args []string) error {
	fs := flag.NewFlagSet("identity-import", flag.ContinueOnError)
	in := fs.String("in", "", "身份 JSON 行文件路径")
	dryRun := fs.Bool("dry-run", false, "只统计不写入")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return errors.New("identity-import 需要 -in=<path>")
	}
	_, admin, err := open()
	if err != nil {
		return err
	}
	defer admin.Close()

	file, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer file.Close()
	summary, err := admin.ImportIdentity(context.Background(), file, *dryRun)
	if err != nil {
		return err
	}
	verb := "已导入"
	if *dryRun {
		verb = "dry-run 将导入"
	}
	fmt.Printf("%s身份数据: %s\n", verb, summary.String())
	fmt.Printf("共 %d 行\n", summary.Rows)
	return nil
}

// --- helpers ----------------------------------------------------------------

func open() (config.Database, *dbadmin.Admin, error) {
	cfg, err := config.LoadDatabase()
	if err != nil {
		return config.Database{}, nil, err
	}
	admin, err := dbadmin.Open(cfg)
	if err != nil {
		return config.Database{}, nil, err
	}
	if !admin.DB.IsMySQL() {
		if _, statErr := os.Stat(cfg.SQLitePath); statErr != nil {
			admin.Close()
			return config.Database{}, nil, fmt.Errorf("SQLite 数据库不存在: %s", cfg.SQLitePath)
		}
	}
	expected, err := admin.DB.ExpectedVersions()
	if err != nil {
		admin.Close()
		return config.Database{}, nil, err
	}
	applied, err := admin.DB.Versions(context.Background())
	if err != nil {
		admin.Close()
		return config.Database{}, nil, err
	}
	if missing := missingVersions(expected, applied); len(missing) > 0 {
		admin.Close()
		return config.Database{}, nil, fmt.Errorf("数据库迁移不完整，缺失版本 %v（先启动一次 API 完成迁移，再运行本命令）", missing)
	}
	return cfg, admin, nil
}

func missingVersions(expected, applied []int) []int {
	present := make(map[int]bool, len(applied))
	for _, v := range applied {
		present[v] = true
	}
	var missing []int
	for _, want := range expected {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	return missing
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(absOrDot(path)), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func absOrDot(path string) string {
	dir := filepath.Dir(path)
	if dir == "" {
		return "."
	}
	return dir
}

func printSnapshot(title string, snap dbadmin.Snapshot) {
	fmt.Printf("== %s (%s) ==\n", title, snap.CapturedAt)
	fmt.Printf("迁移版本: %v\n", snap.Migrations)
	fmt.Printf("家庭数: %d\n", snap.Households)
	fmt.Printf("分录: %d 条 / 行: %d 条 / 借 %d / 贷 %d / 平衡=%t\n",
		snap.Ledger.Entries, snap.Ledger.Lines, snap.Ledger.DebitMinor, snap.Ledger.CreditMinor, snap.Ledger.DebitEqualsCredit)
	fmt.Printf("身份摘要(users sha256): %s\n", snap.UsersDigest)
	printCounts("身份与权限", snap.IdentityTables)
	if len(snap.ConfigTables) > 0 {
		printCounts("保留的个人配置", snap.ConfigTables)
	}
}

func printCounts(title string, counts []dbadmin.TableCount) {
	fmt.Printf("-- %s --\n", title)
	total := int64(0)
	for _, c := range counts {
		if c.Missing {
			fmt.Printf("   %-32s (表不存在)\n", c.Table)
			continue
		}
		total += c.Rows
		fmt.Printf("   %-32s %d\n", c.Table, c.Rows)
	}
	fmt.Printf("   %-32s %d\n", "合计", total)
}
