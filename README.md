# Сервис заказов

Backend-сервис заказов на Go для текущего контроля № 1. Сервис создает заказы, отдает один заказ и список, меняет статус и хранит историю статусов. Данные лежат в памяти процесса и пропадают после перезапуска.

Стек: Go 1.22+, роутер `chi`, логи `log/slog`, документация `swaggo/swag` и Swagger UI, тесты на `testing` и `testify`.

## Запуск

```bash
go mod download
go run ./cmd/api
```

Сервис слушает `:8080`. Адрес меняется переменной окружения `HTTP_ADDR`:

```bash
HTTP_ADDR=:9090 go run ./cmd/api
```

Swagger UI: http://localhost:8080/swagger/index.html, спецификация: http://localhost:8080/swagger/doc.json.

## Проверка

```bash
go test ./...
go test ./internal/service/... -cover
go vet ./...
go build ./...
```

Покрытие пакета `internal/service` 100%. Те же команды есть в `Makefile`:

| Команда | Что делает |
|---|---|
| `make run` | запускает сервис |
| `make test` | запускает все тесты с `-race` |
| `make cover` | покрытие `internal/service` по функциям |
| `make swagger` | перегенерирует `docs/` из аннотаций |
| `make comment-guard` | проверяет, что в Go-коде нет комментариев, кроме swag-аннотаций |
| `make check` | проверка `gofmt` без правки файлов, `comment-guard`, `go vet`, тесты, сборка |

## Структура

```text
cmd/api/main.go                    сборка зависимостей, HTTP-сервер, остановка по SIGINT/SIGTERM
internal/order/                    модель заказа, статусы и переходы, ошибки, интерфейс Repository
internal/service/                  бизнес-логика: валидация, итоговая стоимость, переходы, история
internal/repository/memory/        in-memory реализация Repository
internal/handler/                  маршруты, обработчики, отображение ошибок в HTTP, middleware
docs/                              сгенерированный Swagger (docs.go, swagger.json, swagger.yaml)
```

Запрос проходит путь `handler → service → repository → память`. Сервис зависит только от интерфейса `order.Repository`, реализацию и часы `main` передает в `service.NewOrderService(repository, time.Now)`. Обработчики зависят от интерфейса `handler.OrderService`.

## API

Запросы и ответы в JSON. Цена хранится целым числом в минимальных единицах, например в копейках.

| Метод | Путь | Назначение | Успех |
|---|---|---|---:|
| GET | `/health` | проверка доступности | 200 |
| POST | `/api/v1/orders` | создание заказа | 201 |
| GET | `/api/v1/orders` | список заказов в порядке создания | 200 |
| GET | `/api/v1/orders/{id}` | один заказ | 200 |
| PATCH | `/api/v1/orders/{id}/status` | смена статуса | 200 |
| GET | `/api/v1/orders/{id}/history` | история статусов по времени | 200 |

Создание заказа:

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"items":[{"name":"Keyboard","quantity":1,"price":500000},{"name":"Mouse","quantity":3,"price":150000}]}'
```

```json
{
  "id": 1,
  "items": [
    {"name": "Keyboard", "quantity": 1, "price": 500000},
    {"name": "Mouse", "quantity": 3, "price": 150000}
  ],
  "total_price": 950000,
  "status": "new",
  "created_at": "2026-11-20T07:42:42.726603Z"
}
```

Смена статуса:

```bash
curl -X PATCH http://localhost:8080/api/v1/orders/1/status \
  -H 'Content-Type: application/json' \
  -d '{"status":"processing"}'
```

История:

```json
[
  {"status": "new", "changed_at": "2026-11-20T07:42:42.726603Z"},
  {"status": "processing", "changed_at": "2026-11-20T07:42:42.821146Z"}
]
```

## Правила

При создании проверяется, что позиций хотя бы одна, у каждой непустое название (строка из пробелов считается пустой), `quantity > 0` и `price > 0`. `total_price` равен сумме `price × quantity`. Если сумма не помещается в `int64`, заказ отклоняется с ошибкой валидации.

Новый заказ получает статус `new`, первая запись истории создается вместе с ним. Разрешены переходы `new → processing`, `new → cancelled`, `processing → completed`, `processing → cancelled`. Из `completed` и `cancelled` перейти нельзя. Переход в тот же статус тоже запрещен. Отклоненный переход не меняет заказ и не пишет историю.

## Ошибки

Формат всех ошибок:

```json
{"error": "status transition is not allowed", "code": "INVALID_STATUS_TRANSITION"}
```

| Код ответа | `code` | Когда |
|---:|---|---|
| 400 | `INVALID_JSON` | тело не JSON, лишние поля, неверные типы, больше одного объекта |
| 400 | `VALIDATION_ERROR` | заказ не прошел валидацию, в `error` поле и причина, например `items[0].price must be greater than 0` |
| 400 | `INVALID_ID` | `id` в пути не положительное целое в десятичной записи без знака и ведущих нулей |
| 400 | `INVALID_STATUS` | статуса нет среди `new`, `processing`, `completed`, `cancelled` |
| 404 | `ORDER_NOT_FOUND` | заказа с таким `id` нет |
| 404 | `NOT_FOUND` | неизвестный путь |
| 405 | `METHOD_NOT_ALLOWED` | метод не поддерживается для этого пути, допустимые методы в заголовке `Allow` |
| 409 | `INVALID_STATUS_TRANSITION` | переход статуса запрещен |
| 413 | `BODY_TOO_LARGE` | тело запроса больше 1 МиБ |
| 500 | `INTERNAL_ERROR` | неожиданная ошибка или паника, детали пишутся только в лог |

Предметные ошибки объявлены в `internal/order/errors.go`. Сервис оборачивает их через `%w`, а `handler.writeServiceError` выбирает код через `errors.Is` и `errors.As` в одном месте.

## Логи и middleware

Логи пишутся в stdout в JSON через `log/slog`. На каждый запрос пишется одна строка:

```json
{"time":"2026-11-20T15:15:29.639387+03:00","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/orders/1/history","status":200,"aborted":false,"duration":"58.417µs","request_id":"ac2c5e21fbf4a469"}
```

`request_id` сервис генерирует сам на каждый запрос (16 hex-символов) и отдает клиенту в заголовке `X-Request-ID`. Значение из заголовка клиента не используется. Recovery middleware перехватывает панику, пишет в лог значение и стек и отвечает `500 INTERNAL_ERROR`, сервер продолжает работать. Если обработчик успел начать ответ до паники, middleware обрывает соединение через `http.ErrAbortHandler`, чтобы клиент не принял обрезанный ответ за успешный. В логе в этом случае стоит `response_started: true`, строка лога запроса тоже пишется.

У сервера заданы таймауты: заголовки 5 с, чтение и запись 10 с, простой соединения 60 с. По SIGINT и SIGTERM сервер перестает принимать запросы и до 10 с ждет текущие.

## Допущения

Хранилище живет в памяти одного процесса, ID выдаются подряд с 1 и не переиспользуются. Смена статуса читает заказ, проверяет переход и сохраняет его под мьютексом сервиса, поэтому два одновременных PATCH одного заказа не теряют запись в истории. Мьютекс общий на все заказы. Для одного процесса этого хватает, для общего хранилища нужен условный update в репозитории.

## Тесты

`internal/service/order_service_test.go` содержит table-driven тесты на поддельном репозитории `fakeRepository`. Он реализует `order.Repository`, хранит данные во встроенном `memory.OrderRepository` и умеет возвращать заданную ошибку из любого метода. Часы в тестах подменены, каждый вызов сдвигает время на минуту, поэтому порядок истории проверяется точными значениями. Покрыты:

- успешное создание и расчет итоговой стоимости;
- заказ без позиций, пустое название, нулевые и отрицательные `quantity` и `price`, переполнение суммы;
- получение существующего и отсутствующего заказа, список;
- все допустимые и недопустимые переходы, запрет менять `completed` и `cancelled`, неизвестный статус;
- добавление записей в историю и ее хронологический порядок;
- ошибки репозитория.

Еще есть тесты in-memory репозитория (копии данных, порядок, отсутствующий заказ), table-driven тест HTTP-кодов и кодов ошибок для всех маршрутов на подставном сервисе и тест recovery middleware.

## Swagger

Аннотации лежат над `main` в `cmd/api/main.go` и над обработчиками в `internal/handler/orders.go`. Перегенерация:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/api/main.go -o docs --parseInternal
```
