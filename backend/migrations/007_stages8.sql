-- 8 giai đoạn thi công (thiết kế mới, 18/11/2026): chuyển giá trị cũ của hồ sơ đã có.
UPDATE houses SET stage = 'Ép cọc, đào móng' WHERE stage = 'Đào móng / làm móng';
UPDATE houses SET stage = 'Bàn giao' WHERE stage = 'Đã hoàn công';
-- 'Xây phần thô', 'Hoàn thiện' giữ nguyên tên
