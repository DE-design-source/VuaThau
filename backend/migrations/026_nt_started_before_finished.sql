-- Dự án cũ đăng với "Đã hoàn thành" + năm trước (form Đăng dự án) có ngày bắt đầu = ngày đăng, sau ngày hoàn thành
-- -> app hiện "Ngày -692". Ngày bắt đầu không được muộn hơn ngày hoàn thành.
UPDATE nt_projects SET started_on = finished_on WHERE finished_on IS NOT NULL AND finished_on < started_on;
