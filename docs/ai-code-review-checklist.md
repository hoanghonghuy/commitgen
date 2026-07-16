# Checklist rà soát code sau khi xong task

Áp dụng mọi ngôn ngữ. Chỉ làm bước phụ lục khi diff chạm đúng lớp đó.

---

## Bước 1 — Đúng yêu cầu

- [ ] Đúng scope; không thêm/bớt ngoài yêu cầu
- [ ] Không hiểu sai / làm lệch bản chất
- [ ] Edge case đủ; không chỉ happy path
- [ ] Giả định chỗ mơ hồ hợp lý, có nêu rõ
- [ ] Tên biến/hàm/field đúng domain

## Bước 2 — Convention & style

- [ ] Naming khớp project
- [ ] Cấu trúc thư mục / module khớp pattern hiện có
- [ ] Import/export (hoặc tương đương) nhất quán
- [ ] Format khớp linter/formatter
- [ ] Comment/docstring đúng chuẩn project (nếu có)

## Bước 3 — Đọc & bảo trì

- [ ] Hàm/module không dài / không làm nhiều việc
- [ ] Magic number/string → constant có tên
- [ ] Nesting có thể đơn giản hơn thì đơn giản
- [ ] Người khác đọc được logic

## Bước 4 — DRY / tái sử dụng

- [ ] Không viết mới khi đã có sẵn tương đương
- [ ] Không copy-paste thay vì tách dùng chung
- [ ] Tận dụng utility/helper/constant hiện có
- [ ] Hàm mới chỉ khi cái cũ không đủ
- [ ] Một nguồn sự thật cho cùng dữ liệu/logic
- [ ] Symbol/API/thư viện gọi ra tồn tại đúng (tên, module, version)

## Bước 5 — Thiết kế & luồng

- [ ] Luồng dữ liệu/điều khiển khớp thiết kế
- [ ] Không thiếu/thừa bước; không race / sai thứ tự
- [ ] State nhất quán (không lệch giữa các phần)
- [ ] Async: loading / error / success đúng
- [ ] Không double-submit / gọi thừa hoặc thiếu
- [ ] Use case nghiệp vụ đúng (tạo → sửa → xóa → …)

## Bước 6 — Lỗi & validation

- [ ] Try/catch đúng chỗ; không nuốt lỗi
- [ ] Thông báo lỗi rõ; không lộ stack/query/secret
- [ ] Validate đủ ở mọi ranh giới tin cậy
- [ ] Timeout/retry khi gọi dịch vụ ngoài (nếu có)

## Bước 7 — Bảo mật

- [ ] Không injection (nối chuỗi thủ công)
- [ ] Authz đủ; không đụng dữ liệu ngoài quyền
- [ ] Không hardcode/log secret, token, PII
- [ ] Entry point nhạy cảm có auth/authz
- [ ] Input được validate/sanitize

## Bước 8 — Cấu hình & môi trường

- [ ] Không hardcode config/secret — lấy từ env/config
- [ ] Phân biệt đúng dev / staging / production
- [ ] Cập nhật file mẫu (`.env.example`…) khi thêm biến mới

## Bước 9 — Logging

- [ ] Thao tác quan trọng đủ log để truy vết
- [ ] Không log secret / PII
- [ ] Mức log đúng; không log rác

## Bước 10 — Hiệu năng

- [ ] Không N+1 / I-O trong vòng lặp thừa
- [ ] Không over-fetch
- [ ] Không tính toán / re-render thừa (nếu có UI)
- [ ] Danh sách lớn: phân trang / lazy / stream khi cần

## Bước 11 — Testing

- [ ] Test nhánh chính + edge case quan trọng
- [ ] Đã chạy test/build liên quan
- [ ] Không sửa/xóa test chỉ để “cho pass”
- [ ] Assertion kiểm tra hành vi thật, không mock hình thức

## Bước 12 — Dependency

- [ ] Không thêm lib khi đã có giải pháp tương đương
- [ ] Khai báo đúng manifest; version hợp lý
- [ ] Không xung đột phiên bản rõ ràng

## Bước 13 — Git

- [ ] Diff đúng phạm vi task
- [ ] Không sót file debug / tạm / log thừa
- [ ] Commit message đúng thay đổi (nếu có)

## Bước 14 — Regression

- [ ] Tính năng cũ liên quan vẫn đúng
- [ ] Sửa shared code không gây side-effect xấu

## Bước 15 — Tài liệu & comment

- [ ] Logic phức tạp: comment *why*, không chỉ *what*
- [ ] Đổi API/props/schema → cập nhật docs liên quan

---

## Phụ lục — chỉ khi có trong diff

### A. UI

- [ ] Đúng thiết kế/yêu cầu; responsive nếu trong scope
- [ ] Loading / empty / error đủ
- [ ] Validation khớp phía tin cậy
- [ ] Cleanup timer/subscription; không leak
- [ ] Component tái sử dụng được; props rõ; không hardcode thừa
- [ ] A11y cơ bản (label, bàn phím, contrast)
- [ ] Kiểm với dữ liệu rỗng / lớn / sai định dạng

### B. API / hợp đồng công khai

- [ ] Method/route/command/status khớp convention
- [ ] Request/response khớp bên gọi
- [ ] Mã lỗi phù hợp convention project
- [ ] Logic đúng layer (controller/service/repository… theo kiến trúc hiện có)
- [ ] Sửa surface cũ → backward compatible

### C. Database / data

- [ ] Schema/migration đúng convention đặt tên
- [ ] Index cho filter/join hay dùng
- [ ] FK / cascade đúng và an toàn
- [ ] Transaction đúng chỗ cần toàn vẹn dữ liệu
- [ ] Concurrent update an toàn (lock/versioning…) nếu trong scope
- [ ] Import trùng: skip/update/merge đúng yêu cầu
- [ ] Migrate/seed không phá dữ liệu thật ngoài ý muốn

### D. Multi-tenant

- [ ] Mọi query lọc đúng tenant; không leak chéo
