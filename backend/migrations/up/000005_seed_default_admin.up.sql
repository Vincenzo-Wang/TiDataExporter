-- =============================================
-- Migration: 000005_seed_default_admin.up.sql
-- Description: 初始化默认管理员（幂等）
-- =============================================

INSERT INTO admins (username, password_hash, email, role, status)
SELECT 'admin', '$2y$10$WdQsrT8PpgRYm1Q6zlzZTOtbu8LwcpTKMThRdyiWC5t6uk9oZ5zjC', 'admin@example.com', 'admin', 1
WHERE NOT EXISTS (
    SELECT 1 FROM admins WHERE username = 'admin'
);
