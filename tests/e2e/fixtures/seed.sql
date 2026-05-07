CREATE DATABASE IF NOT EXISTS testdata;
USE testdata;
DROP TABLE IF EXISTS world_data;
CREATE TABLE world_data (
  base_country JSON,
  birth_rate INT,
  co2 INT,
  gdp INT,
  date_time DATETIME(6),
  timestamp_value BIGINT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO world_data (base_country, birth_rate, co2, gdp, date_time, timestamp_value) VALUES
  (JSON_OBJECT('name', 'United States', 'code', 'US'), 12, 5416, 21433, '2026-03-17 21:00:00.000000', 1774825200000),
  (JSON_OBJECT('name', 'China',         'code', 'CN'), 11, 11680, 14280, '2026-03-17 21:30:00.000000', 1774827000000),
  (JSON_OBJECT('name', 'India',         'code', 'IN'), 17, 2654, 2870, '2026-03-17 22:00:00.000000', 1774828800000),
  (JSON_OBJECT('name', 'Germany',       'code', 'DE'), 9, 696, 3845, '2026-03-17 22:30:00.000000', 1774830600000),
  (JSON_OBJECT('name', 'Brazil',        'code', 'BR'), 14, 466, 1840, '2026-03-17 23:00:00.000000', 1774832400000),
  (JSON_OBJECT('name', 'Japan',         'code', 'JP'), 7, 1162, 5081, '2026-03-17 23:30:00.000000', 1774834200000),
  (JSON_OBJECT('name', 'United Kingdom', 'code', 'GB'), 11, 351, 2827, '2026-03-18 00:00:00.000000', 1774836000000),
  (JSON_OBJECT('name', 'France',        'code', 'FR'), 11, 306, 2715, '2026-03-18 00:30:00.000000', 1774837800000),
  (JSON_OBJECT('name', 'Canada',        'code', 'CA'), 10, 565, 1736, '2026-03-18 01:00:00.000000', 1774839600000);
