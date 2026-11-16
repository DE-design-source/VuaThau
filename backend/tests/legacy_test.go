package tests

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/scrypt"
	_ "modernc.org/sqlite"

	"vuathau/backend/internal/legacy"
	"vuathau/backend/internal/model"
	"vuathau/backend/internal/service"
)

// Lược đồ SQLite của máy chủ Python cũ (app.py SCHEMA + các cột thêm sau).
const legacySchema = `
create table users(id integer primary key, phone text unique not null, name text not null,
  role text not null, pw text not null, bank text not null default '',
  created_at text not null default (datetime('now','localtime')), google text not null default '', email text not null default '',
  locked integer not null default 0);
create table batches(id integer primary key, status text not null default 'da_chi', total integer not null, note text not null default '',
  created_by integer, created_at text not null default (datetime('now','localtime')), ctv_id integer, receipt text not null default '');
create table houses(id integer primary key, ctv_id integer not null, status text not null default 'nhap',
  address text not null default '', lat real, lng real, contractor_name text not null default '', contractor_phone text not null default '',
  owner_name text not null default '', owner_phone text not null default '', floors integer, stage text not null default '',
  note text not null default '', dup text not null default '', amount integer not null default 0, ai_state text not null default '',
  ai_json text not null default '', batch_id integer, created_at text not null default (datetime('now','localtime')),
  drive text not null default '');
create table photos(id integer primary key, house_id integer not null, sha256 text unique not null, dhash text not null,
  exif_time text not null default '', ai_time text not null default '', lat real, lng real, address text not null default '',
  is_site integer, dup_of integer);
create table log(id integer primary key, house_id integer, batch_id integer, user_id integer, action text not null,
  note text not null default '', at text not null default (datetime('now','localtime')));
create table settings(k text primary key, v text not null);
create table ai_cache(sha256 text primary key, json text not null, at text not null default (datetime('now','localtime')));
create table adjust(id integer primary key, ctv_id integer not null, amount integer not null, note text not null,
  batch_id integer, created_by integer, created_at text not null default (datetime('now','localtime')));`

// legacyHash: giống hash_pw của bản Python (salt hex + scrypt n=2^14, r=8, p=1, 64 byte).
func legacyHash(t *testing.T, pw string) string {
	salt := "0123456789abcdef0123456789abcdef"
	k, err := scrypt.Key([]byte(pw), []byte(salt), 1<<14, 8, 1, 64)
	if err != nil {
		t.Fatal(err)
	}
	return salt + "$" + hex.EncodeToString(k)
}

func TestImportLegacy(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	src := t.TempDir()
	dbPath := filepath.Join(src, "nghiemthu.db")
	lite, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lite.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	day := time.Now().In(service.VN).Format("2006-01-02")
	pw := legacyHash(t, "matkhau-cu")
	stmts := []string{
		fmt.Sprintf(`insert into users(id,phone,name,role,pw,bank,created_at) values
			(1,'admin','Quản trị','ceo','%s','','2026-11-01 08:00:00'),
			(2,'ketoan','Kế toán','ketoan','%s','','2026-11-01 08:00:00'),
			(7,'0901234567','Minh Khôi','ctv','%s','Vietcombank · 0123456789 · MINH KHOI','2026-11-02 09:30:00')`, pw, pw, pw),
		`insert into batches(id,status,total,note,created_by,created_at,ctv_id) values (3,'da_chi',85000,'FT999',2,'2026-11-10 10:00:00',7)`,
		`insert into houses(id,ctv_id,status,address,lat,lng,contractor_name,contractor_phone,owner_name,floors,stage,amount,ai_json,batch_id,created_at)
		 values (5,7,'da_chi','Tam Long',10.8,106.7,'Xây dựng SON','0902818188','chủ cũ',3,'Hoàn thiện',35000,'{"stage":"Hoàn thiện"}',3,'2026-11-05 14:13:00'),
		        (6,7,'cho_ceo','Q1',null,null,'','','',null,'',50000,'',null,'2026-11-06 14:13:00'),
		        (8,7,'cho_duyet','Q2',null,null,'','','',null,'',35000,'',null,'2026-11-07 14:13:00')`,
		fmt.Sprintf(`insert into photos(id,house_id,sha256,dhash,exif_time,lat,lng) values (11,5,'%064d','ffffffffffffffff','%s 09:00',10.8,106.7)`, 1, day),
		`insert into log(id,house_id,batch_id,user_id,action,note,at) values
			(1,5,null,7,'Tạo hồ sơ','','2026-11-05 14:13:00'),(2,5,null,1,'Duyệt','','2026-11-06 09:00:00'),
			(3,null,3,2,'Đã thanh toán','1 hồ sơ','2026-11-10 10:00:00'),(4,99,null,7,'Tạo hồ sơ','','2026-11-05 14:13:00')`,
		`insert into adjust(id,ctv_id,amount,note,batch_id,created_by) values (1,7,50000,'Thưởng',3,2),(2,7,-10000,'Trừ',null,2)`,
		`insert into settings(k,v) values ('pay_min','3'),('price_min','40000')`,
		fmt.Sprintf(`insert into ai_cache(sha256,json) values ('%064d','{"photos":[]}')`, 1),
	}
	for _, s := range stmts {
		if _, err := lite.Exec(s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	lite.Close()
	// File ảnh gốc + ảnh thu nhỏ, biên lai
	_ = os.MkdirAll(filepath.Join(src, "anh"), 0o755)
	_ = os.MkdirAll(filepath.Join(src, "bienlai"), 0o755)
	sha := fmt.Sprintf("%064d", 1)
	img := photo(t, 42)
	_ = os.WriteFile(filepath.Join(src, "anh", sha), img, 0o644)
	_ = os.WriteFile(filepath.Join(src, "anh", sha+"_s.jpg"), img, 0o644)

	dataDir := e.data
	st, err := legacy.Import(context.Background(), e.db, legacy.Options{SQLitePath: dbPath, DataDir: dataDir})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if st["users"] != 3 || st["houses"] != 3 || st["photos"] != 1 || st["logs"] != 3 || st["adjustments"] != 2 || st["batches"] != 1 {
		t.Fatalf("stats = %v", st)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "anh", sha)); err != nil {
		t.Fatalf("ảnh chưa được chép: %v", err)
	}
	// Chuyển lần 2: từ chối vì CSDL đích đã có dữ liệu
	if _, err := legacy.Import(context.Background(), e.db, legacy.Options{SQLitePath: dbPath, DataDir: dataDir}); err == nil {
		t.Fatal("import lần 2 phải báo lỗi")
	}

	// Đăng nhập bằng mật khẩu cũ (scrypt); lần sau vẫn đăng nhập được (đã nâng lên bcrypt)
	ctv := e.login(t, "0901234567", "matkhau-cu")
	e.login(t, "0901234567", "matkhau-cu")
	var hash string
	_ = e.db.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE id = 7`).Scan(&hash)
	if hash[:2] != "$2" {
		t.Fatalf("mật khẩu chưa được nâng lên bcrypt: %s", hash)
	}
	admin := e.login(t, "admin", "matkhau-cu")
	if admin.User.Role != model.RoleAdmin {
		t.Fatalf("ceo phải thành admin, được %s", admin.User.Role)
	}

	// Tiền của CTV: đã nhận 35.000 + thưởng 50.000; chờ trả: hồ sơ cho_ceo (về da_duyet) 50.000 - trừ 10.000
	r := e.do(t, http.MethodGet, "/api/v1/ctv/income", ctv.AccessToken, nil)
	var inc service.Income
	decodeData(t, r, &inc)
	if inc.Money.Paid != 85000 || inc.Money.Owed != 40000 || inc.Money.Approved != 1 || len(inc.Pays) != 1 || inc.Pays[0].Ref != "FT999" {
		t.Fatalf("income: %s", r.Raw)
	}
	// Hồ sơ cũ: ảnh xem được, lịch sử còn, cấu hình chuyển sang
	r = e.do(t, http.MethodGet, "/api/v1/houses/5", ctv.AccessToken, nil)
	var det service.HouseDetail
	decodeData(t, r, &det)
	if len(det.Photos) != 1 || len(det.Logs) != 3 || det.House.BatchID == nil || len(det.AIFields) != 1 {
		t.Fatalf("house 5: %s", r.Raw)
	}
	if ph := e.getRaw(t, "/api/v1/photos/11", ctv.AccessToken); ph.Status != http.StatusOK {
		t.Fatalf("ảnh cũ: %d", ph.Status)
	}
	r = e.do(t, http.MethodGet, "/api/v1/config", "", nil)
	var cfg service.Config
	decodeData(t, r, &cfg)
	if cfg.PayMin != 3 || cfg.Pay[5] != 40000 {
		t.Fatalf("settings: %s", r.Raw)
	}
	// id mới tiếp nối id cũ
	newCTV := e.register(t, "CTV mới")
	if newCTV.User.ID <= 7 {
		t.Fatalf("id mới %d phải lớn hơn id cũ", newCTV.User.ID)
	}
	// Admin thấy hồ sơ cũ chờ duyệt
	r = e.do(t, http.MethodGet, "/api/v1/admin/queue", admin.AccessToken, nil)
	var q struct {
		Rows []model.House `json:"rows"`
	}
	decodeData(t, r, &q)
	if len(q.Rows) != 1 || q.Rows[0].ID != 8 {
		t.Fatalf("queue: %s", r.Raw)
	}
}
