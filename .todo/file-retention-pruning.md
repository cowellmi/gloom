# File retention / pruning for SD card logs and sensor data

**severity:** feature

Daily rotation is implemented with date-stamped files in `GLOOM/` on the SD card: `GLOOM/20260214.CSV` for sensor recordings and `GLOOM/20260214.LOG` for log entries. Old files are never deleted. Add configurable retention periods with separate settings for logs and sensor recordings:

- `log_retain_days` — number of days to keep log files (e.g. 7). Diagnostic logs are high-volume and low-value once reviewed; a researcher may want to keep only a few days.
- `data_retain_days` — number of days to keep sensor recordings. Sensor data is the primary scientific output and may need to be kept indefinitely (`0` = keep forever) until manually retrieved from the SD card.

Implementation notes:
- On startup or daily rotation, compute expected old filenames (`{dir}/{YYYYMMDD}{ext}`) going back beyond the retention window and call `card.Remove` for each. This avoids FAT directory listing (which is limited in fatfs) by using predictable date-stamped names.
- `card.Remove` already exists for this purpose.
