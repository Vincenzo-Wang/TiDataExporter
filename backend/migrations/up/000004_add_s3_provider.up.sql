-- 添加 S3 存储厂商字段
ALTER TABLE s3_configs ADD COLUMN provider VARCHAR(20) DEFAULT 'aws' COMMENT '存储厂商: aws, aliyun' AFTER name;

-- 更新已有记录的 provider 字段
UPDATE s3_configs SET provider = 'aws' WHERE provider IS NULL OR provider = '';
