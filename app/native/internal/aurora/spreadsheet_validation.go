package aurora

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"nekoaurora.local/native/internal/documents"
)

func checkSheet(r io.ReaderAt, size int64, ext string) verdict {
	limit := int64(documents.MaxOfficePackageBytes)
	if ext == ".csv" {
		limit = 2 << 20
	} else if ext != ".xlsx" {
		return verdict{why: "định dạng bảng tính chưa được kiểm tra"}
	}
	if size <= 0 || size > limit {
		return verdict{why: fmt.Sprintf("chưa kiểm chứng: file trống hoặc vượt giới hạn %s", kbSize(limit))}
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(io.NewSectionReader(r, 0, size), raw); err != nil {
		return verdict{why: "chưa đọc được toàn bộ file bảng tính"}
	}
	if ext == ".csv" {
		return checkCSV(raw)
	}
	summary, err := documents.InspectXLSX(raw)
	if err != nil {
		return verdict{why: "XLSX chưa đạt kiểm tra cấu trúc: " + err.Error()}
	}
	if summary.MaxPopulatedRows < 2 {
		return verdict{why: "XLSX chưa có trang tính với ít nhất 2 dòng chứa dữ liệu"}
	}
	note := fmt.Sprintf("XLSX %d trang, tối đa %d dòng dữ liệu/trang; đạt kiểm tra cấu trúc", summary.Sheets, summary.MaxPopulatedRows)
	if summary.FormulaCells != 0 {
		note += fmt.Sprintf("; %d ô công thức chưa tính lại, %d ô chưa có kết quả cache", summary.FormulaCells, summary.UncachedFormulaCells)
	}
	return verdict{true, note}
}

func checkCSV(raw []byte) verdict {
	if !utf8.Valid(raw) {
		return verdict{why: "CSV chưa có mã hóa UTF-8 hợp lệ"}
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	bestRows, bestColumns := 0, 0
	for _, separator := range []rune{',', ';', '\t'} {
		reader := csv.NewReader(bytes.NewReader(raw))
		reader.Comma = separator
		reader.ReuseRecord = true
		rows, columns := 0, 0
		valid := true
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				valid = false
				break
			}
			if columns == 0 {
				columns = len(record)
				if columns < 2 {
					valid = false
					break
				}
			}
			nonempty := false
			for _, value := range record {
				nonempty = nonempty || strings.TrimSpace(value) != ""
			}
			if nonempty {
				rows++
			}
		}
		if valid && rows >= 2 && columns > bestColumns {
			bestRows, bestColumns = rows, columns
		}
	}
	if bestColumns == 0 {
		return verdict{why: "CSV cần ít nhất 2 dòng dữ liệu, 2 cột và mọi bản ghi có cùng số cột; dấu nháy phải đóng đầy đủ"}
	}
	return verdict{true, fmt.Sprintf("CSV %d dòng × %d cột; đã đọc toàn bộ cấu trúc", bestRows, bestColumns)}
}
