-- +goose Up

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Реестр известных узлов сети
CREATE TABLE nodes (
                       id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       name          TEXT NOT NULL UNIQUE,
                       base_url      TEXT NOT NULL UNIQUE,
                       shared_secret TEXT NOT NULL,
                       status        TEXT NOT NULL DEFAULT 'active',
                       created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Пользователи — локальные (is_local=true) и удалённые (is_local=false)
CREATE TABLE users (
                       id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       global_id      TEXT NOT NULL UNIQUE,        -- alice@node-a
                       local_username TEXT,                         -- только для локальных
                       home_node      TEXT NOT NULL,
                       display_name   TEXT NOT NULL DEFAULT '',
                       bio            TEXT NOT NULL DEFAULT '',
                       password_hash  TEXT,                         -- NULL для remote users
                       is_local       BOOLEAN NOT NULL DEFAULT true,
                       created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- уникальность username только среди локальных пользователей
CREATE UNIQUE INDEX users_local_username_idx
    ON users (local_username) WHERE is_local = true;

-- Посты — локальные и реплики с других узлов
CREATE TABLE posts (
                       id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       global_id   TEXT NOT NULL UNIQUE,   -- post:alice@node-a:01ARYZ6S41
                       author_id   UUID NOT NULL REFERENCES users(id),
                       origin_node TEXT NOT NULL,          -- узел где пост был создан
                       content     TEXT NOT NULL,
                       visibility  TEXT NOT NULL DEFAULT 'public',
                       status      TEXT NOT NULL DEFAULT 'active',  -- active | deleted
                       version     INT  NOT NULL DEFAULT 1,          -- для разрешения конфликтов
                       hide_likes BOOLEAN NOT NULL DEFAULT false,
                       created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
                       updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX posts_author_idx ON posts (author_id);

-- Подписки — локальные и на удалённых пользователей
CREATE TABLE follows (
                         id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                         follower_user_id      UUID NOT NULL REFERENCES users(id),
                         target_global_user_id TEXT NOT NULL,   -- global_id цели (может быть remote)
                         target_node           TEXT NOT NULL,   -- узел цели
                         status                TEXT NOT NULL DEFAULT 'active',
                         created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
                         UNIQUE (follower_user_id, target_global_user_id)
);

-- Материализованная лента каждого пользователя
CREATE TABLE feed_items (
                            id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                            owner_user_id  UUID NOT NULL REFERENCES users(id),
                            post_global_id TEXT NOT NULL,   -- global_id поста (может быть с другого узла)
                            source_node    TEXT NOT NULL,
                            created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
                            UNIQUE (owner_user_id, post_global_id)
);
-- индекс для быстрой выборки ленты по убыванию даты
CREATE INDEX feed_items_owner_idx
    ON feed_items (owner_user_id, created_at DESC);

-- Исходящие федеративные события (outbox pattern)
CREATE TABLE federation_outbox (
                                   id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                   event_id      TEXT NOT NULL UNIQUE,   -- ulid, глобально уникальный
                                   target_node   TEXT NOT NULL,
                                   event_type    TEXT NOT NULL,          -- post.created | post.updated | post.deleted | user.followed
                                   payload       JSONB NOT NULL,
                                   status        TEXT NOT NULL DEFAULT 'pending',  -- pending | delivered | failed
                                   retry_count   INT  NOT NULL DEFAULT 0,
                                   next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                   created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- partial index — worker читает только pending события, индекс маленький и быстрый
CREATE INDEX outbox_pending_idx
    ON federation_outbox (status, next_retry_at)
    WHERE status = 'pending';

-- Уже обработанные входящие события — гарантия идемпотентности
CREATE TABLE processed_events (
                                  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                  event_id     TEXT NOT NULL UNIQUE,   -- тот же event_id что в outbox отправителя
                                  source_node  TEXT NOT NULL,
                                  processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- Лайки постов
CREATE TABLE likes (
                       id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       user_id        UUID NOT NULL REFERENCES users(id),
                       post_global_id TEXT NOT NULL,
                       created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
                       UNIQUE (user_id, post_global_id)
);
CREATE INDEX likes_post_idx ON likes (post_global_id);

-- Комментарии к постам
CREATE TABLE comments (
                          id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                          global_id      TEXT NOT NULL UNIQUE,  -- comment:author@node:ULID
                          post_global_id TEXT NOT NULL,
                          author_id      UUID NOT NULL REFERENCES users(id),
                          origin_node    TEXT NOT NULL,
                          content        TEXT NOT NULL,
                          status         TEXT NOT NULL DEFAULT 'active',
                          created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX comments_post_idx ON comments (post_global_id, created_at DESC);

-- Диалоги (личные переписки между двумя пользователями)
CREATE TABLE conversations (
                               id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                               participant_a   TEXT NOT NULL,  -- global_id
                               participant_b   TEXT NOT NULL,  -- global_id
                               last_message_at TIMESTAMPTZ,
                               created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                               UNIQUE (participant_a, participant_b)
);
CREATE INDEX conversations_a_idx ON conversations (participant_a);
CREATE INDEX conversations_b_idx ON conversations (participant_b);

-- Сообщения в диалогах
CREATE TABLE messages (
                          id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                          global_id       TEXT NOT NULL UNIQUE,  -- msg:sender@node:ULID
                          conversation_id UUID NOT NULL REFERENCES conversations(id),
                          sender_id       UUID NOT NULL REFERENCES users(id),
                          content         TEXT NOT NULL,
                          status          TEXT NOT NULL DEFAULT 'sent',  -- sent | delivered | read
                          created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX messages_conv_idx ON messages (conversation_id, created_at DESC);


-- +goose Down
DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS federation_outbox;
DROP TABLE IF EXISTS feed_items;
DROP TABLE IF EXISTS follows;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS likes;
