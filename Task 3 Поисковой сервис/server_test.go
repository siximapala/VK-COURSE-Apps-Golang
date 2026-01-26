package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Тестовая структура для мока XML
func TestSearchServer_Integration(t *testing.T) {
	originalDataset := datasetFile
	defer func() { datasetFile = originalDataset }()

	// Создаем временный тестовый XML файл
	testXML := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <row>
        <id>1</id>
        <first_name>Jasin</first_name>
        <last_name>Smith</last_name>
        <age>25</age>
        <about>Software engineer</about>
        <gender>male</gender>
    </row>
    <row>
        <id>2</id>
        <first_name>Jonathan</first_name>
        <last_name>Doe</last_name>
        <age>35</age>
        <about>Manager who loves coding</about>
        <gender>male</gender>
    </row>
    <row>
        <id>3</id>
        <first_name>Alice</first_name>
        <last_name>Johnson</last_name>
        <age>30</age>
        <about>Designer</about>
        <gender>female</gender>
    </row>
</root>`

	// Создаем временный файл
	tmpfile, err := os.CreateTemp("", "test_dataset_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testXML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Устанавливаем тестовый файл
	datasetFile = tmpfile.Name()

	// Прямые вызовы SearchServer через httptest
	t.Run("Simple request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?limit=5", nil)
		rr := httptest.NewRecorder()
		SearchServer(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
		}
		var users []User
		if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
			t.Fatalf("unmarshal users: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("expected 3 users, got %d", len(users))
		}
	})

	t.Run("With query 'on'", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?limit=5&query=on", nil)
		rr := httptest.NewRecorder()
		SearchServer(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
		}
		var users []User
		if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
			t.Fatalf("unmarshal users: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users with 'on', got %d", len(users))
		}
	})

	t.Run("Sort by age desc", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?limit=5&order_field=Age&order_by=-1&query=on", nil)
		rr := httptest.NewRecorder()
		SearchServer(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
		}
		var users []User
		if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
			t.Fatalf("unmarshal users: %v", err)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
		if users[0].Age != 35 {
			t.Errorf("expected first user age 35, got %d", users[0].Age)
		}
	})
}

// Тест для parseIntParam
func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		hasError bool
	}{
		{"empty string", "", 0, false},
		{"valid number", "42", 42, false},
		{"invalid number", "abc", 0, true},
		{"negative number", "-10", -10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseIntParam(tt.input)

			if tt.hasError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.hasError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %d, want %d", result, tt.expected)
			}
		})
	}
}

// Тест SearchServer напрямую
func TestSearchServer_Direct(t *testing.T) {
	// Подменяем datasetFile на тестовый
	originalDataset := datasetFile

	// Создаем временный тестовый XML
	testXML := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <row>
        <id>1</id>
        <first_name>Test</first_name>
        <last_name>User</last_name>
        <age>30</age>
        <about>Test user</about>
        <gender>male</gender>
    </row>
</root>`

	tmpfile, err := os.CreateTemp("", "test_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testXML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	datasetFile = tmpfile.Name()
	defer func() { datasetFile = originalDataset }()

	// тестируем разные проносы в параметрах
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"valid request", "?limit=5&offset=0", http.StatusOK},
		{"negative limit", "?limit=-1", http.StatusBadRequest},
		{"negative offset", "?offset=-1", http.StatusBadRequest},
		{"invalid order_by", "?order_by=abc", http.StatusBadRequest},
		{"bad order_field", "?order_field=Invalid", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/search"+tt.query, nil)
			w := httptest.NewRecorder()

			SearchServer(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// Тест на фильтрацию
func TestSearchServer_Filtering(t *testing.T) {
	originalDataset := datasetFile
	defer func() { datasetFile = originalDataset }()

	testXML := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <row>
        <id>1</id>
        <first_name>John</first_name>
        <last_name>Doe</last_name>
        <age>25</age>
        <about>Developer</about>
        <gender>male</gender>
    </row>
    <row>
        <id>2</id>
        <first_name>Jane</first_name>
        <last_name>Smith</last_name>
        <age>30</age>
        <about>Engineer</about>
        <gender>female</gender>
    </row>
</root>`

	tmpfile, err := os.CreateTemp("", "filter_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(testXML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	datasetFile = tmpfile.Name()

	// тест запрос с query, который не найдет ничего
	req := httptest.NewRequest("GET", "/search?query=nonexistent", nil)
	w := httptest.NewRecorder()

	SearchServer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var users []User
	if err := json.NewDecoder(w.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}

	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

// тест на пагинацию
func TestSearchServer_Pagination(t *testing.T) {
	originalDataset := datasetFile
	defer func() { datasetFile = originalDataset }()

	// Создаем XML с 5 пользователями через string builder: Builder накапливает байты, создает строку один раз, а не конкатенирует много раз
	// и сохраняет новую строку в памяти
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?><root>`)
	for i := 1; i <= 5; i++ {
		builder.WriteString(`<row>
            <id>` + strconv.Itoa(i) + `</id>
            <first_name>User</first_name>
            <last_name>` + strconv.Itoa(i) + `</last_name>
            <age>` + strconv.Itoa(20+i) + `</age>
            <about>User ` + strconv.Itoa(i) + `</about>
            <gender>male</gender>
        </row>`)
	}
	builder.WriteString(`</root>`)

	tmpfile, err := os.CreateTemp("", "pagination_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(builder.String())); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	datasetFile = tmpfile.Name()

	tests := []struct {
		name   string
		limit  int
		offset int
		want   int
	}{
		{"limit 2, offset 0", 2, 0, 2},
		{"limit 2, offset 2", 2, 2, 2},
		{"limit 10, offset 0", 10, 0, 5}, // всего 5 записей
		{"limit 0, offset 0", 0, 0, 5},   // limit=0 значит все
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := "/search?limit=" + strconv.Itoa(tt.limit) + "&offset=" + strconv.Itoa(tt.offset)
			req := httptest.NewRequest("GET", query, nil)
			w := httptest.NewRecorder()

			SearchServer(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var users []User
			if err := json.NewDecoder(w.Body).Decode(&users); err != nil {
				t.Fatal(err)
			}

			if len(users) != tt.want {
				t.Errorf("got %d users, want %d", len(users), tt.want)
			}
		})
	}
}

func TestSearchServer_DefaultOrderFieldAndSorting(t *testing.T) {
	original := datasetFile
	defer func() { datasetFile = original }()

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <row>
	<id>1</id>
	<age>30</age>
	<first_name>B</first_name>
	<last_name>Beta</last_name>
	<about>x</about>
	<gender>male</gender>
  </row>
  <row>
	<id>2</id>
	<age>25</age>
	<first_name>A</first_name>
	<last_name>Alpha</last_name>
	<about>y</about>
	<gender>female</gender>
  </row>
</root>`

	tmp, err := os.CreateTemp("", "deford_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte(xml))
	tmp.Close()

	datasetFile = tmp.Name()

	// запрос с order_by=1, но без order_field - должно сортировать по Name по умолчанию
	req := httptest.NewRequest("GET", "/?order_by=1&limit=10", nil)
	rr := httptest.NewRecorder()
	SearchServer(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var users []User
	if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users got %d", len(users))
	}
	// Имена должны быть в алфавитном порядке проверяем
	if users[0].Name >= users[1].Name {
		t.Fatalf("expected ascending by name, got %q, %q", users[0].Name, users[1].Name)
	}
}

func TestSearchServer_SortByIdDescWithPagination(t *testing.T) {
	original := datasetFile
	defer func() { datasetFile = original }()

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <row><id>1</id><first_name>X</first_name><last_name>One</last_name><age>20</age><about>a</about><gender>m</gender></row>
  <row><id>3</id><first_name>Y</first_name><last_name>Two</last_name><age>22</age><about>b</about><gender>f</gender></row>
  <row><id>2</id><first_name>Z</first_name><last_name>Three</last_name><age>21</age><about>c</about><gender>m</gender></row>
</root>`
	tmp, err := os.CreateTemp("", "sortid_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte(xml))
	tmp.Close()

	datasetFile = tmp.Name()

	// сортируем по Id по убыванию, пропускаем 1, лимит 1 - должен вернуться пользователь с Id=2
	req := httptest.NewRequest("GET", "/?order_by=-1&order_field=Id&offset=1&limit=1", nil)
	rr := httptest.NewRecorder()
	SearchServer(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var users []User
	if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user got %d", len(users))
	}
	if users[0].ID != 2 {
		t.Fatalf("expected id 2 got %d", users[0].ID)
	}
}

func TestSearchServer_FileOpenError(t *testing.T) {
	original := datasetFile
	defer func() { datasetFile = original }()

	datasetFile = "nonexistent_file_hopefully"
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	SearchServer(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 got %d", rr.Code)
	}
}

func TestSearchServer_OffsetBeyondLenAfterSorting(t *testing.T) {
	original := datasetFile
	defer func() { datasetFile = original }()

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <row><id>1</id><first_name>A</first_name><last_name>a</last_name><age>20</age><about>a</about><gender>m</gender></row>
  <row><id>2</id><first_name>B</first_name><last_name>b</last_name><age>21</age><about>b</about><gender>f</gender></row>
</root>`
	tmp, err := os.CreateTemp("", "offs_*.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write([]byte(xml))
	tmp.Close()

	datasetFile = tmp.Name()

	// смещение больше чем количество записей значит должен вернуться пустой результат
	req := httptest.NewRequest("GET", "/?order_by=1&order_field=Age&offset=10&limit=5", nil)
	rr := httptest.NewRecorder()
	SearchServer(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var users []User
	if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected 0 users got %d", len(users))
	}
}
