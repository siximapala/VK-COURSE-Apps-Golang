package main

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// datasetFile можно менять в тестах
var datasetFile = "dataset.xml"

type xmlRow struct {
	ID        int    `xml:"id"`
	Age       int    `xml:"age"`
	FirstName string `xml:"first_name"`
	LastName  string `xml:"last_name"`
	About     string `xml:"about"`
	Gender    string `xml:"gender"`
}

// корневая структура XML: массив записей row, в которых хранятся пользователи
type xmlRoot struct {
	Rows []xmlRow `xml:"row"`
}

func parseIntParam(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	return strconv.Atoi(v)
}

func SearchServer(w http.ResponseWriter, r *http.Request) {
	// читаем параметры
	//сколько нужно возвращаемых значений
	limit, err := parseIntParam(r.FormValue("limit"))
	if err != nil || limit < 0 {
		http.Error(w, "limit must be >= 0", http.StatusBadRequest)
		return
	}
	//смещение пропускает N первых записей
	offset, err := parseIntParam(r.FormValue("offset"))
	if err != nil || offset < 0 {
		http.Error(w, "offset must be >= 0", http.StatusBadRequest)
		return
	}
	//поле сортировки
	orderField := r.FormValue("order_field")
	//направление сортировки
	orderByStr := r.FormValue("order_by")
	// по умолчанию без сортировки
	orderBy := 0
	if orderByStr != "" {
		orderBy, err = strconv.Atoi(orderByStr)
		if err != nil {
			http.Error(w, "order_by must be integer", http.StatusBadRequest)
			return
		}
	}

	if orderField == "" {
		orderField = "Name"
	}
	ok := orderField == "Id" || orderField == "Age" || orderField == "Name"
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(SearchErrorResponse{Error: ErrorBadOrderField})
		return
	}

	query := r.FormValue("query")

	// читаем xml потоком: парсим по-строчно элементы <row>
	f, err := os.Open(datasetFile)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	dec := xml.NewDecoder(f)

	// Если сортировки нет (orderBy==0), можем странично отдавать результат без хранения всего набора
	users := make([]User, 0)

	idx := 0
	collected := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			http.Error(w, "cant unpack xml", http.StatusInternalServerError)
			return
		}
		//
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "row" {
			// Обрабатываем элемент <row>
			var xr xmlRow
			if err := dec.DecodeElement(&xr, &start); err != nil {
				// обработка ошибки
			}
			// формируем наш элементы User
			u := User{
				ID:     xr.ID,
				Name:   strings.TrimSpace(xr.FirstName + " " + xr.LastName),
				Age:    xr.Age,
				About:  xr.About,
				Gender: xr.Gender,
			}

			// фильтрация по query
			if query != "" && !(strings.Contains(u.Name, query) || strings.Contains(u.About, query)) {
				continue
			}

			// Вариант 1: сортировка не требуется, применяем offset limit прямо при чтении
			if orderBy == 0 {
				// пропускаем пока не дойдем до offset
				if idx < offset {
					idx++
					continue
				}
				users = append(users, u)
				collected++
				// если установлен лимит и собрали достаточно то можно завершить чтение файла
				if limit > 0 && collected >= limit {
					// мы можем прекратить декодирование, т.к. ответ собран
					// закрываем файл и прерываем цикл
					goto RESPOND
				}
			} else {
				// при сортировке собираем все подходящие записи, чтобы ПОТОМ уже с ними поработать
				users = append(users, u)
			}
		}
	}
RESPOND:

	// сортировка (уже после того как все записи прочитаны делаем сортировку, т.к. иначе
	// limit и offset могли дать другой набор записей)
	if orderBy != 0 {
		asc := orderBy == 1
		sort.SliceStable(users, func(i, j int) bool {
			switch orderField {
			case "Id":
				if asc {
					return users[i].ID < users[j].ID
				}
				return users[i].ID > users[j].ID
			case "Age":
				if asc {
					return users[i].Age < users[j].Age
				}
				return users[i].Age > users[j].Age
			default: // Name
				if asc {
					return users[i].Name < users[j].Name
				}
				return users[i].Name > users[j].Name
			}
		})
	}

	var result []User
	if orderBy == 0 {
		result = users
	} else {
		// пагинация offset limit после сортировки
		start := offset
		if start > len(users) {
			start = len(users)
		}
		end := len(users)
		if limit > 0 {
			end = start + limit
			if end > len(users) {
				end = len(users)
			}
		}
		result = users[start:end]
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}
