package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var (
	player *Player
	room   *Room
	rooms  map[string]*Room
)

var actionsAliases = map[string]string{
	"поднять":       "take",
	"надеть":        "take",
	"получить":      "take",
	"взять":         "take",
	"забрать":       "take",
	"осмотреться":   "look",
	"посмотреть":    "look",
	"идти":          "go",
	"пойти":         "go",
	"применить":     "use",
	"использовать":  "use",
	"выйти из игры": "exit",
}

type ActionType int

const (
	ActionTake ActionType = iota
	ActionLook
	ActionGo
	ActionUse
)

type Action struct {
	action          ActionType
	requirement     string
	afterCommentary string
	OnSuccess       func()
}

type GameObject struct {
	Name    string
	Actions []Action
}

type Inventory struct {
	Items []string
}

type Room struct {
	Inventory
	Reactions   map[string]string
	Description string
	Objects     map[string]GameObject
}

type Player struct {
	Inventory
}

// Функция для поиска объекта
func getObject(room *Room, name string) (*GameObject, bool) {
	if obj, ok := room.Objects[name]; ok {
		return &obj, true
	}
	return nil, false
}

// Вспомогательные функции
func updateBedroomDescription() {
	bedroom := rooms["комната"]
	items := bedroom.Items

	// Создаем карту для удобной проверки
	itemMap := make(map[string]bool)
	for _, item := range items {
		itemMap[item] = true
	}

	// Формируем описание
	var descriptionParts []string

	if itemMap["ключи"] && itemMap["конспекты"] {
		descriptionParts = append(descriptionParts, "на столе: ключи, конспекты")
	} else if itemMap["ключи"] {
		descriptionParts = append(descriptionParts, "на столе: ключи")
	} else if itemMap["конспекты"] {
		descriptionParts = append(descriptionParts, "на столе: конспекты")
	}

	if itemMap["рюкзак"] {
		descriptionParts = append(descriptionParts, "на стуле: рюкзак")
	}

	// Если ничего нет
	if len(descriptionParts) == 0 {
		descriptionParts = append(descriptionParts, "пустая комната")
	}

	bedroom.Description = strings.Join(descriptionParts, ", ") + ". можно пройти - коридор"
}

func updateKitchenDescription() {
	kitchen := rooms["кухня"]
	if hasAllItems() {
		kitchen.Description = "ты находишься на кухне, на столе: чай, надо идти в универ. можно пройти - коридор"
	}
}

func hasAllItems() bool {
	hasKeys := false
	hasNotes := false
	for _, item := range player.Items {
		if item == "ключи" {
			hasKeys = true
		}
		if item == "конспекты" {
			hasNotes = true
		}
	}
	return hasKeys && hasNotes
}

// pickItem - взять предмет из комнаты
func (player *Player) pickItem(item string, room *Room) (bool, string) {
	// Проверяем, есть ли предмет в комнате
	itemIndex := -1
	for i, roomItem := range room.Items {
		if roomItem == item {
			itemIndex = i
			break
		}
	}

	if itemIndex == -1 {
		return false, "нет такого"
	}

	// Особый случай для рюкзака
	if item == "рюкзак" {
		room.Items = append(room.Items[:itemIndex], room.Items[itemIndex+1:]...)
		player.Items = append(player.Items, item)
		updateBedroomDescription()
		return true, "вы надели: рюкзак"
	}

	// Для других предметов проверяем наличие рюкзака
	hasBackpack := false
	for _, playerItem := range player.Items {
		if playerItem == "рюкзак" {
			hasBackpack = true
			break
		}
	}

	if !hasBackpack {
		return false, "некуда класть"
	}

	room.Items = append(room.Items[:itemIndex], room.Items[itemIndex+1:]...)
	player.Items = append(player.Items, item)
	updateBedroomDescription()
	updateKitchenDescription()
	return true, "предмет добавлен в инвентарь: " + item
}

// проверяем есть ли такой предмет в инвентаре
func (player *Player) hasItem(item string) bool {
	for _, playerItem := range player.Items {
		if playerItem == item {
			return true
		}
	}
	return false
}

// Универсальная обработка действий с объектами
func handleObjectAction(obj *GameObject, actionType ActionType, itemToUse string) (string, bool) {
	for _, action := range obj.Actions {
		if action.action == actionType {
			// Для ActionGo проверяем requirement (открыта ли дверь)
			if actionType == ActionGo {
				if action.requirement == "" {
					action.OnSuccess()
					return action.afterCommentary, true
				}
				return "дверь закрыта", false
			}
			// Для ActionUse проверяем, совпадает ли requirement с используемым предметом
			if actionType == ActionUse {
				if action.requirement == itemToUse {
					action.OnSuccess()
					return action.afterCommentary, true
				}
			}
		}
	}
	return "", false
}

func resolveReaction(msg string, player *Player, room *Room) string {
	parts := strings.Split(strings.TrimSpace(msg), " ")
	if len(parts) == 0 {
		return "Введите команду"
	}

	verb := parts[0]
	result, ok := actionsAliases[verb]

	if !ok {
		return "неизвестная команда"
	}

	switch result {
	case "take":
		if len(parts) < 2 {
			return "укажите предмет"
		}
		item := parts[1]
		success, response := player.pickItem(item, room)
		if success {
			return response
		}
		return response

	case "look":
		return room.Description

	case "go":
		if len(parts) < 2 {
			return "укажите направление"
		}
		direction := parts[1]

		// Специальная обработка для "улица" в коридоре
		if direction == "улица" && room == rooms["коридор"] {
			direction = "дверь"
		}

		obj, ok := getObject(room, direction)
		if !ok {
			return "нет пути в " + parts[1]
		}

		response, success := handleObjectAction(obj, ActionGo, "")
		if success {
			return response
		}
		return response

	case "use":
		if len(parts) < 3 {
			return "укажите предмет и объект"
		}
		itemToUse := parts[1]
		objectName := parts[2]

		// Проверяем, есть ли предмет у игрока
		if !player.hasItem(itemToUse) {
			return "нет предмета в инвентаре - " + itemToUse
		}

		obj, ok := getObject(room, objectName)
		if !ok {
			return "не к чему применить"
		}

		response, success := handleObjectAction(obj, ActionUse, itemToUse)
		if success {
			return response
		}

		// Если действие не удалось, возвращаем предмет обратно
		player.Items = append(player.Items, itemToUse)
		return "не к чему применить"

	case "exit":
		return "Спасибо за игру!"

	default:
		return "неизвестная команда"
	}
}

func main() {
	initGame()

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Добро пожаловать в квест!")
	fmt.Println("Доступные команды: взять, осмотреться, идти, применить, выход")
	fmt.Println("Например: 'осмотреться', 'взять ключи', 'идти коридор', 'применить ключи дверь'")
	fmt.Println("Введите 'выйти из игры' для выхода")
	fmt.Println()

	for {
		fmt.Print("> ")

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Ошибка чтения ввода:", err)
			continue
		}

		// Убираем лишние пробелы и переводы строк
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}
		result := handleCommand(input)
		fmt.Println(result)
		fmt.Println()

		// Проверяем, не завершилась ли игра
		if result == "Спасибо за игру!" {
			break
		}
	}
}

func initGame() {
	player = &Player{
		Inventory: Inventory{Items: []string{}},
	}

	// Создаем комнаты
	kitchen := &Room{
		Inventory:   Inventory{Items: []string{}},
		Description: "ты находишься на кухне, на столе: чай, надо собрать рюкзак и идти в универ. можно пройти - коридор",
		Objects:     make(map[string]GameObject),
	}

	corridor := &Room{
		Inventory:   Inventory{Items: []string{}},
		Description: "ничего интересного. можно пройти - кухня, комната, улица",
		Objects:     make(map[string]GameObject),
	}

	bedroom := &Room{
		Inventory:   Inventory{Items: []string{"ключи", "конспекты", "рюкзак"}},
		Description: "на столе: ключи, конспекты, на стуле: рюкзак. можно пройти - коридор",
		Objects:     make(map[string]GameObject),
	}

	street := &Room{
		Inventory:   Inventory{Items: []string{}},
		Description: "на улице весна. можно пройти - домой",
		Objects:     make(map[string]GameObject),
	}

	// Инициализация проходов для кухни
	kitchen.Objects["коридор"] = GameObject{
		Name: "коридор",
		Actions: []Action{
			{
				action:          ActionGo,
				requirement:     "",
				afterCommentary: "ничего интересного. можно пройти - кухня, комната, улица",
				OnSuccess: func() {
					room = corridor
				},
			},
		},
	}

	// Инициализация проходов для коридора
	corridor.Objects["кухня"] = GameObject{
		Name: "кухня",
		Actions: []Action{
			{
				action:          ActionGo,
				requirement:     "",
				afterCommentary: "кухня, ничего интересного. можно пройти - коридор",
				OnSuccess: func() {
					room = kitchen
					kitchen.Description = "ты находишься на кухне, на столе: чай, надо собрать рюкзак и идти в универ. можно пройти - коридор"
					if hasAllItems() {
						kitchen.Description = "ты находишься на кухне, на столе: чай, надо идти в универ. можно пройти - коридор"
					}
				},
			},
		},
	}

	corridor.Objects["комната"] = GameObject{
		Name: "комната",
		Actions: []Action{
			{
				action:          ActionGo,
				requirement:     "",
				afterCommentary: "ты в своей комнате. можно пройти - коридор",
				OnSuccess: func() {
					room = bedroom
					updateBedroomDescription()
				},
			},
		},
	}

	// Дверь в коридоре - УНИВЕРСАЛЬНЫЙ ПОДХОД
	corridor.Objects["дверь"] = GameObject{
		Name: "дверь",
		Actions: []Action{
			{
				action:          ActionUse,
				requirement:     "ключи",
				afterCommentary: "дверь открыта",
				OnSuccess: func() {
					// После применения ключей, обновляем дверь
					corridor.Objects["дверь"] = GameObject{
						Name: "дверь",
						Actions: []Action{
							{
								action:          ActionGo,
								requirement:     "",
								afterCommentary: "на улице весна. можно пройти - домой",
								OnSuccess: func() {
									room = street
								},
							},
						},
					}
				},
			},
			{
				action:          ActionGo,
				requirement:     "ключи",
				afterCommentary: "на улице весна. можно пройти - домой",
				OnSuccess: func() {
					room = street
				},
			},
		},
	}

	// Инициализация проходов для комнаты
	bedroom.Objects["коридор"] = GameObject{
		Name: "коридор",
		Actions: []Action{
			{
				action:          ActionGo,
				requirement:     "",
				afterCommentary: "ничего интересного. можно пройти - кухня, комната, улица",
				OnSuccess: func() {
					room = corridor
				},
			},
		},
	}

	// Инициализация проходов для улицы
	street.Objects["домой"] = GameObject{
		Name: "домой",
		Actions: []Action{
			{
				action:          ActionGo,
				requirement:     "",
				afterCommentary: "ничего интересного. можно пройти - кухня, комната, улица",
				OnSuccess: func() {
					room = corridor
				},
			},
		},
	}

	// Сохраняем комнаты
	rooms = map[string]*Room{
		"кухня":   kitchen,
		"коридор": corridor,
		"комната": bedroom,
		"улица":   street,
	}

	// Начинаем игру на кухне
	room = kitchen
}

func handleCommand(command string) string {
	return resolveReaction(command, player, room)
}
