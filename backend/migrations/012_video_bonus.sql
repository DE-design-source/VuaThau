-- Thưởng hồ sơ có video (19/11/2026): hồ sơ được trả tiền và có ít nhất 1 video cộng thêm settings 'video_bonus'
-- (mặc định 10.000đ, admin sửa ở Cấu hình). Hồ sơ gửi duyệt từ nay tính tự động.
-- Chạy 1 lần (khi chưa có dòng 'video_bonus'): cộng thưởng cho hồ sơ ĐANG CHỜ DUYỆT đã có video và đang được trả tiền;
-- hồ sơ đã duyệt / đã thanh toán / từ chối giữ nguyên.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM settings WHERE k = 'video_bonus') THEN
        UPDATE houses h SET amount = h.amount + 10000
        WHERE h.status = 'cho_duyet' AND h.amount > 0 AND EXISTS (SELECT 1 FROM videos v WHERE v.house_id = h.id);
        INSERT INTO settings (k, v) VALUES ('video_bonus', 10000);
    END IF;
END $$;
