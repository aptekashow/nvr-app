# NVR App

Самописный NVR (Network Video Recorder) с непрерывной записью и возможностью скачать любой произвольный фрагмент **без перекодирования**.

## Возможности

- Авторизация (JWT)
- Сетка превью камер с обновлением каждые 10 секунд
- Прямой эфир (HLS) с зумом колёсиком мыши
- Непрерывная запись в MP4 (`-c:v copy` где возможно)
- Экспорт любого временного фрагмента через `ffmpeg -c copy` (без перекодирования)
- Добавление / удаление камер
- Старт / стоп записи

> Страница «Архив» пока заглушка — будет реализована следующей итерацией.

## Стек

| Компонент     | Технология              |
|---------------|-------------------------|
| Backend       | Go + Echo               |
| Streaming     | MediaMTX                |
| Recording     | FFmpeg                  |
| Frontend      | React + TypeScript + Vite + hls.js |

## Быстрый старт

### Требования
- Docker + Docker Compose
- Node.js 18+

### Запуск

```bash
# 1. Backend + MediaMTX
cd deploy
docker compose up -d --build

# 2. Frontend
cd ../frontend
npm install
npm run dev
```

Открыть: **http://localhost:5173**

Логин по умолчанию: `admin` / `admin`

## Структура проекта

```
nvr-app/
├── backend/                 # Go API
│   ├── cmd/server/
│   └── internal/
│       ├── api/             # HTTP handlers
│       ├── config/
│       ├── models/
│       ├── store/           # In-memory (легко заменить на SQLite/Postgres)
│       └── stream/          # Превью, запись, экспорт фрагментов
├── frontend/                # React
│   └── src/pages/
│       ├── Login.tsx
│       ├── Dashboard.tsx    # Сетка превью
│       └── Live.tsx         # Эфир + зум
└── deploy/
    ├── docker-compose.yml
    └── mediamtx.yml
```

## API (кратко)

| Метод | Путь                          | Описание                    |
|-------|-------------------------------|-----------------------------|
| POST  | `/api/login`                  | Авторизация                 |
| GET   | `/api/cameras`                | Список камер + статусы      |
| POST  | `/api/cameras`                | Добавить камеру             |
| DELETE| `/api/cameras/:id`            | Удалить камеру              |
| POST  | `/api/cameras/:id/record/start` | Начать запись             |
| POST  | `/api/cameras/:id/record/stop`  | Остановить запись         |
| POST  | `/api/export`                 | Вырезать фрагмент (скачать) |
| GET   | `/api/previews/:id.jpg`       | Превью-кадр                 |

## Как работает запись и экспорт

- Запись идёт **непрерывно** в MP4-файл (один файл на сутки или с суффиксом времени).
- При экспорте фрагмента вызывается:

```bash
ffmpeg -ss <start> -to <end> -i recording.mp4 -c copy -avoid_negative_ts make_zero out.mp4
```

То есть **без перекодирования**.

## Дальнейшие планы

- [ ] Полноценная страница Архива с таймлайном
- [ ] Корректный поиск фрагмента между несколькими файлами за день
- [ ] Авто-публикация камер в MediaMTX через API
- [ ] SQLite / PostgreSQL вместо in-memory
- [ ] WebRTC для меньшей задержки
- [ ] Детекция движения (опционально)

## Лицензия

MIT
