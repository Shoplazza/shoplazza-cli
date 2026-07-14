package migrate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/testenv"
)

// fixtures 构造：在 t.TempDir 下铺 v1 形态文件
// v1 config.json: {"current_account":"Alice@Co.com","store_domain":"us.myshoplazza.com"}
// v1 auth.json:   {"account":"Alice@Co.com","granted_scopes":["read_product"],
//                  "stores":{"us.myshoplazza.com":{"store_id":"100001"},"cn.myshoplazza.com":{"store_id":"100002"}}}
// v1 keychain:    writeLegacyEntry(uat/partner)（复用 T2 测试助手，keychain.SetLegacy）

type fixtureOpts struct {
	storeDomain string
	stores      map[string]map[string]string
}

type fixtureOpt func(*fixtureOpts)

func withStoreDomain(domain string) fixtureOpt {
	return func(o *fixtureOpts) { o.storeDomain = domain }
}

func withStores(domains ...string) fixtureOpt {
	return func(o *fixtureOpts) {
		if o.stores == nil {
			o.stores = map[string]map[string]string{}
		}
		for i, d := range domains {
			o.stores[d] = map[string]string{"store_id": "10000" + string(rune('1'+i))}
		}
	}
}

// layV1Fixture lays a v1 config.json + auth.json + legacy keychain entries
// under dir, isolating the OS config dir so keychain reads/writes stay
// confined to the test. Returns the config.json path.
func layV1Fixture(t *testing.T, dir string, opts ...fixtureOpt) string {
	t.Helper()
	testenv.IsolateConfigDir(t)

	o := fixtureOpts{}
	for _, apply := range opts {
		apply(&o)
	}

	cp := filepath.Join(dir, "config.json")
	v1cfg := map[string]string{"current_account": "Alice@Co.com"}
	if o.storeDomain != "" {
		v1cfg["store_domain"] = o.storeDomain
	}
	writeJSON(t, cp, v1cfg)

	stores := o.stores
	if stores == nil {
		stores = map[string]map[string]string{}
	}
	v1auth := map[string]any{
		"account":        "Alice@Co.com",
		"granted_scopes": []string{"read_product"},
		"stores":         stores,
	}
	writeJSON(t, filepath.Join(dir, "auth.json"), v1auth)

	if err := keychain.SetLegacy(keychain.ShoplazzaCliService, "uat", "legacy-uat"); err != nil {
		t.Fatalf("SetLegacy uat: %v", err)
	}
	if err := keychain.SetLegacy(keychain.ShoplazzaCliService, "partner", "legacy-partner"); err != nil {
		t.Fatalf("SetLegacy partner: %v", err)
	}

	return cp
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestRun_FreshInstall_NoV1Files(t *testing.T) {
	testenv.IsolateConfigDir(t)
	cp := filepath.Join(t.TempDir(), "config.json")
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	if cfg.ConfigVersion != 2 || len(cfg.Profiles) != 0 {
		t.Fatalf("fresh init: %+v", cfg)
	}
}

func TestRun_AllStores_BecomeProfiles_CurrentFromLegacy(t *testing.T) {
	dir := t.TempDir()
	cp := layV1Fixture(t, dir, withStoreDomain("us.myshoplazza.com"), withStores("us.myshoplazza.com", "cn.myshoplazza.com"))
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	if cfg.ConfigVersion != 2 || cfg.CurrentProfile != "us" || len(cfg.Profiles) != 2 {
		t.Fatalf("profile: %+v", cfg) // 每个 v1 店建 Profile；current 跟 legacy store_domain
	}
	byName := map[string]core.ProfileConfig{}
	for _, p := range cfg.Profiles {
		byName[p.Name] = p
	}
	if p, ok := byName["us"]; !ok || p.StoreID != "100001" || p.StoreDomain != "us.myshoplazza.com" {
		t.Fatalf("us profile: %+v", byName)
	}
	if p, ok := byName["cn"]; !ok || p.StoreID != "100002" || p.StoreDomain != "cn.myshoplazza.com" {
		t.Fatalf("cn profile: %+v", byName)
	}
	if cfg.Accounts[0].Name != "alice@co.com" { // 邮箱小写归一
		t.Fatalf("account: %+v", cfg.Accounts)
	}
	// uat/partner 已迁到新命名，可用新 Get 读到
	if v, _ := keychain.Get(keychain.ShoplazzaCliService, "account:alice@co.com:uat"); v != "legacy-uat" {
		t.Fatal("uat not migrated")
	}
	// store token 不迁移；Get 对不存在的条目返回 ("", nil)，不是 error（T2 契约）
	if v, err := keychain.Get(keychain.ShoplazzaCliService, "profile:us:store"); err != nil || v != "" {
		t.Fatalf("store token must NOT be migrated, got %q, %v", v, err)
	}
}

// 无 legacy store_domain 时：唯一的迁移店自动成为 current（对齐 profile add
// 首个 profile 的行为）；多店则留空，由用户 profile use 挑选。
func TestRun_StoresOnly_SingleBecomesCurrent(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir(), withStores("us.myshoplazza.com"))
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	if len(cfg.Profiles) != 1 || cfg.CurrentProfile != "us" || cfg.Profiles[0].StoreID != "100001" {
		t.Fatalf("single store: %+v", cfg)
	}
}

func TestRun_StoresOnly_MultipleNoCurrent(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir(), withStores("us.myshoplazza.com", "cn.myshoplazza.com"))
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	if len(cfg.Profiles) != 2 || cfg.CurrentProfile != "" {
		t.Fatalf("multi store must not guess a current: %+v", cfg)
	}
}

// 已存在的 v2 凭证绝不能被 legacy 覆盖：migrate 可能在 config.json 意外缺失
// 时误跑（v2 keychain 仍在），此时 legacy uat/partner 是旧登录的陈值，
// 覆盖会把有效凭证换成已撤销的（2026-07-14 真实事故）。
func TestRun_DoesNotClobberExistingV2Credentials(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir(), withStoreDomain("us.myshoplazza.com"))
	// 先有一个 v2 登录留下的新 UAT/partner，再触发 migrate。
	if err := keychain.Set(keychain.ShoplazzaCliService, "account:alice@co.com:uat", "fresh-uat"); err != nil {
		t.Fatal(err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, "account:alice@co.com:partner", "fresh-partner"); err != nil {
		t.Fatal(err)
	}
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	if v, _ := keychain.Get(keychain.ShoplazzaCliService, "account:alice@co.com:uat"); v != "fresh-uat" {
		t.Fatalf("v2 uat clobbered by legacy: %q", v)
	}
	if v, _ := keychain.Get(keychain.ShoplazzaCliService, "account:alice@co.com:partner"); v != "fresh-partner" {
		t.Fatalf("v2 partner clobbered by legacy: %q", v)
	}
}

// 派生名冲突（shop.myshoplaza.com 与 shop.stg.myshoplaza.com 同缩 "shop"）：
// 后者获得 -2 后缀；域名排序保证结果确定。
func TestRun_Stores_NameCollisionGetsSuffix(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir(), withStores("shop.myshoplaza.com", "shop.stg.myshoplaza.com"))
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	byName := map[string]string{}
	for _, p := range cfg.Profiles {
		byName[p.Name] = p.StoreDomain
	}
	if byName["shop"] != "shop.myshoplaza.com" || byName["shop-2"] != "shop.stg.myshoplaza.com" {
		t.Fatalf("collision naming: %+v", byName)
	}
}

func TestRun_Idempotent(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir(), withStoreDomain("us.myshoplazza.com"))
	_ = Run(cp)
	before, _ := os.ReadFile(cp)
	if err := Run(cp); err != nil { // 第二次：configVersion=2 短路
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cp)
	if !bytes.Equal(before, after) {
		t.Fatal("second run must be a no-op")
	}
}

func TestRun_PreservesV1FilesAndWritesBak(t *testing.T) {
	dir := t.TempDir()
	cp := layV1Fixture(t, dir, withStoreDomain("us.myshoplazza.com"))
	_ = Run(cp)
	mustExist(t, filepath.Join(dir, "auth.json")) // v1 元数据保留
	mustExist(t, cp+".v1.bak")                    // 覆写前备份
	var bak map[string]string
	_ = json.Unmarshal(mustRead(t, cp+".v1.bak"), &bak)
	if bak["store_domain"] != "us.myshoplazza.com" {
		t.Fatal(".v1.bak must hold the v1 content")
	}
}

// MIG-02：仅登录未选店——迁 Account/凭证，不建 Profile
func TestRun_AccountOnlyNoStore(t *testing.T) {
	cp := layV1Fixture(t, t.TempDir()) // 无 withStoreDomain
	if err := Run(cp); err != nil {
		t.Fatal(err)
	}
	cfg, _ := core.LoadConfig(cp)
	if cfg.ConfigVersion != 2 || len(cfg.Profiles) != 0 || len(cfg.Accounts) != 1 {
		t.Fatalf("account-only: %+v", cfg)
	}
	if v, _ := keychain.Get(keychain.ShoplazzaCliService, "account:alice@co.com:uat"); v != "legacy-uat" {
		t.Fatal("uat must migrate even without a store")
	}
}

// MIG-05：损坏的 v1 config 必须明确报错、零半迁移状态
func TestRun_CorruptConfigFailsLoudly(t *testing.T) {
	testenv.IsolateConfigDir(t)
	dir := t.TempDir()
	cp := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cp, []byte("{not json"), 0o600)
	if err := Run(cp); err == nil {
		t.Fatal("corrupt config must error")
	}
	if _, err := os.Stat(cp + ".v1.bak"); err == nil {
		t.Fatal("must not write .bak on failure")
	}
	raw, _ := os.ReadFile(cp)
	if string(raw) != "{not json" {
		t.Fatal("must not touch the corrupt file")
	}
}

func TestRun_ConcurrentFirstRun_MigratesOnce(t *testing.T) {
	// 两个 goroutine 同时 Run；锁 + double-check 下均无错且终态一致。
	// （flock 跨 fd 语义在同进程内同样互斥，等价于双进程；E2E 层另有真实双进程用例）
	cp := layV1Fixture(t, t.TempDir(), withStoreDomain("us.myshoplazza.com"))
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = Run(cp) }(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	cfg, _ := core.LoadConfig(cp)
	if cfg.ConfigVersion != 2 || len(cfg.Profiles) != 1 {
		t.Fatalf("exactly-once: %+v", cfg)
	}
}
