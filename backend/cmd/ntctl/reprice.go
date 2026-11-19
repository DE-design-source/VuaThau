package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
	"vuathau/backend/internal/service"
)

// reprice: tính lại tiền hồ sơ đang chờ duyệt theo đúng luật của máy chủ (ảnh đạt, loại chụp, thưởng video),
// dùng sau khi sửa dữ liệu bằng tay (vd gỡ ảnh gộp nhầm). Hồ sơ không ở trạng thái chờ duyệt: bỏ qua.
// Ghi lịch sử hồ sơ khi tiền đổi; note: lý do ghi kèm.
//
//	ntctl reprice -note "lý do" <mã hồ sơ> [<mã hồ sơ> ...]
func reprice(ctx context.Context, pool *pgxpool.Pool, note string, args []string) {
	houses := repository.NewHouseRepository(pool)
	photos := repository.NewPhotoRepository(pool)
	st, err := repository.NewSettingsRepository(pool).Get(ctx)
	if err != nil {
		fail(err)
	}
	for _, a := range args {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			fail(fmt.Errorf("mã hồ sơ không hợp lệ: %s", a))
		}
		h, err := houses.Get(ctx, id)
		if err != nil {
			fail(fmt.Errorf("hồ sơ #%d: %w", id, err))
		}
		if h.Status != model.StatusPending {
			fmt.Printf("#%d: bỏ qua (trạng thái %s)\n", id, h.Status)
			continue
		}
		ps, err := photos.ListByHouse(ctx, id)
		if err != nil {
			fail(err)
		}
		vs, err := photos.Videos(ctx, id)
		if err != nil {
			fail(err)
		}
		valid := service.FillProblems(ps, time.Now())
		amount := st.WithVideo(st.AmountFor(valid, h.EffScope()), len(vs))
		fmt.Printf("#%d: %d ảnh (%d đạt), %d video: %s -> %s\n", id, len(ps), valid, len(vs), service.Money(h.Amount), service.Money(amount))
		if amount == h.Amount {
			continue
		}
		if err := houses.SetAmount(ctx, id, amount); err != nil {
			fail(err)
		}
		msg := fmt.Sprintf("Tính lại tiền: %s -> %s (%d ảnh đạt)", service.Money(h.Amount), service.Money(amount), valid)
		if note != "" {
			msg += ". " + note
		}
		if _, err := pool.Exec(ctx, `INSERT INTO house_logs (house_id, action, note) VALUES ($1, $2, $3)`, id, "Sửa số tiền", msg); err != nil {
			fail(err)
		}
	}
}
