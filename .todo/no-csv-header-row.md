# No CSV header row in data files

**severity:** low

Data files are written as bare CSV (`timestamp,device,label,value,unit`) but there's no header row. Adding a header on file creation would make the CSVs self-documenting for researchers working with the data offline.
