-- SĐT phụ của nhà thầu (24/11/2026): bảng hiệu công trình có thể ghi số khác số đăng nhập (VD Trịnh Bình 0907165296
-- có công trình ghi Hồng Phát Group 0973234050). Admin gán; app nhà thầu thấy công trình khớp số chính hoặc số phụ.
ALTER TABLE nt_profiles ADD COLUMN IF NOT EXISTS alt_phones TEXT[] NOT NULL DEFAULT '{}';
