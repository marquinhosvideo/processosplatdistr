SET FOREIGN_KEY_CHECKS = 0;  -- ciclo teams↔people↔users; sessão dedicada da migração

CREATE TABLE IF NOT EXISTS org_units (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code       VARCHAR(32)  NOT NULL,
  name       VARCHAR(200) NOT NULL,
  active     BOOLEAN      NOT NULL DEFAULT 1,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_org_units_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS job_titles (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code       VARCHAR(32)  NOT NULL,
  name       VARCHAR(200) NOT NULL,
  active     BOOLEAN      NOT NULL DEFAULT 1,
  source     VARCHAR(8)   NOT NULL DEFAULT 'erp',
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_job_titles_code (code),
  CONSTRAINT ck_job_titles_source CHECK (source IN ('erp','manual'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS teams (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id       BIGINT UNSIGNED NOT NULL,
  code              VARCHAR(32)  NOT NULL,
  name              VARCHAR(200) NOT NULL,
  manager_person_id BIGINT UNSIGNED NULL,          -- gestor declarado (CSV ERP ou Admin); compõe ManagedTeams
  active            BOOLEAN      NOT NULL DEFAULT 1,
  source            VARCHAR(8)   NOT NULL DEFAULT 'erp',
  created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_teams_code (org_unit_id, code),
  KEY ix_teams_manager (manager_person_id),
  KEY ix_teams_org_active (org_unit_id, active),
  CONSTRAINT ck_teams_source CHECK (source IN ('erp','manual')),
  CONSTRAINT fk_teams_org     FOREIGN KEY (org_unit_id)       REFERENCES org_units(id),
  CONSTRAINT fk_teams_manager FOREIGN KEY (manager_person_id) REFERENCES people(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS people (
  id                          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  org_unit_id                 BIGINT UNSIGNED NOT NULL,
  team_id                     BIGINT UNSIGNED NULL,
  employee_number             VARCHAR(32)  NOT NULL,     -- matrícula ERP
  full_name                   VARCHAR(200) NOT NULL,
  email                       VARCHAR(254) NULL,
  email_norm                  VARCHAR(254) NULL,         -- lower(trim(email))
  job_title_id                BIGINT UNSIGNED NULL,
  erp_manager_person_id       BIGINT UNSIGNED NULL,
  manager_override_person_id  BIGINT UNSIGNED NULL,      -- RF-AUTH-03
  manager_override_reason     VARCHAR(500) NULL,
  manager_override_by         BIGINT UNSIGNED NULL,
  manager_override_at         DATETIME(3) NULL,
  effective_manager_person_id BIGINT UNSIGNED
      AS (COALESCE(manager_override_person_id, erp_manager_person_id)) STORED,
  cost_center                 VARCHAR(32) NULL,
  hired_on                    DATE NULL,
  active                      BOOLEAN NOT NULL DEFAULT 1,
  source                      VARCHAR(8) NOT NULL DEFAULT 'erp',
  synced_at                   DATETIME(3) NULL,
  created_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at                  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_people_employee (employee_number),
  UNIQUE KEY uq_people_email (email_norm),
  KEY ix_people_manager (effective_manager_person_id, active),
  KEY ix_people_team (team_id, active),
  KEY ix_people_org_name (org_unit_id, full_name),
  CONSTRAINT ck_people_source   CHECK (source IN ('erp','manual')),
  CONSTRAINT ck_people_override CHECK (manager_override_person_id IS NULL OR manager_override_reason IS NOT NULL),
  CONSTRAINT fk_people_org      FOREIGN KEY (org_unit_id)                REFERENCES org_units(id),
  CONSTRAINT fk_people_team     FOREIGN KEY (team_id)                    REFERENCES teams(id),
  CONSTRAINT fk_people_title    FOREIGN KEY (job_title_id)               REFERENCES job_titles(id),
  CONSTRAINT fk_people_erp_mgr  FOREIGN KEY (erp_manager_person_id)      REFERENCES people(id),
  CONSTRAINT fk_people_ovr_mgr  FOREIGN KEY (manager_override_person_id) REFERENCES people(id),
  CONSTRAINT fk_people_ovr_by   FOREIGN KEY (manager_override_by)        REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  oidc_issuer   VARCHAR(255) NOT NULL,
  oidc_subject  VARCHAR(255) NOT NULL,
  email         VARCHAR(254) NOT NULL,
  email_norm    VARCHAR(254) NOT NULL,
  display_name  VARCHAR(200) NOT NULL,
  person_id     BIGINT UNSIGNED NULL,
  person_link   VARCHAR(8) NULL,                          -- como o vínculo foi feito
  last_roles    SET('manager','head','admin') NOT NULL DEFAULT '',
  last_login_at DATETIME(3) NULL,
  disabled_at   DATETIME(3) NULL,                         -- bloqueio pelo Admin
  disabled_by   BIGINT UNSIGNED NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_users_subject (oidc_issuer, oidc_subject),
  UNIQUE KEY uq_users_person (person_id),
  KEY ix_users_email (email_norm),
  CONSTRAINT ck_users_link    CHECK (person_link IS NULL OR person_link IN ('employee','email','manual')),
  CONSTRAINT fk_users_person   FOREIGN KEY (person_id)   REFERENCES people(id),
  CONSTRAINT fk_users_disabler FOREIGN KEY (disabled_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS sessions (
  token_hash            BINARY(32) NOT NULL,              -- sha256(token do cookie)
  user_id               BIGINT UNSIGNED NOT NULL,
  roles                 SET('manager','head','admin') NOT NULL,
  refresh_token_enc     BLOB NULL,                        -- seal(AES-GCM), AAD "rt|<hash>"
  id_token_enc          BLOB NULL,                        -- seal, só p/ id_token_hint no logout
  created_at            DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at          DATETIME(3) NOT NULL,
  absolute_expires_at   DATETIME(3) NOT NULL,
  roles_checked_at      DATETIME(3) NOT NULL,
  recheck_failing_since DATETIME(3) NULL,
  ip                    VARCHAR(45)  NULL,
  user_agent            VARCHAR(255) NULL,
  PRIMARY KEY (token_hash),
  KEY ix_sessions_user (user_id),
  KEY ix_sessions_abs (absolute_expires_at),
  KEY ix_sessions_seen (last_seen_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS app_settings (               -- configs e feature flags ("feature.*"); defaults em Go
  setting_key VARCHAR(100) NOT NULL,
  value       JSON NOT NULL,
  updated_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (setting_key),
  CONSTRAINT fk_settings_user FOREIGN KEY (updated_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS audit_log (                  -- append-only; a app nunca faz UPDATE/DELETE
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  occurred_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  org_unit_id   BIGINT UNSIGNED NULL,
  actor_user_id BIGINT UNSIGNED NULL,                     -- NULL = sistema/job
  action        VARCHAR(64) NOT NULL,                     -- ex.: candidate.transition, quarter.close
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     BIGINT UNSIGNED NULL,
  subject_type  VARCHAR(16) NULL,                         -- titular afetado
  subject_id    BIGINT UNSIGNED NULL,
  before_json   JSON NULL,                                -- só campos plain; redacted = {"changed":true}
  after_json    JSON NULL,
  reason        VARCHAR(1000) NULL,                       -- só ações administrativas (override, anonimização)
  request_id    CHAR(16) NULL,
  ip            VARCHAR(45) NULL,
  PRIMARY KEY (id),
  KEY ix_audit_entity (entity_type, entity_id, occurred_at),
  KEY ix_audit_subject (subject_type, subject_id),
  KEY ix_audit_actor (actor_user_id, occurred_at),
  KEY ix_audit_action (action, occurred_at),
  KEY ix_audit_time (occurred_at),
  CONSTRAINT ck_audit_subject CHECK (subject_type IS NULL OR subject_type IN ('candidate','person')),
  CONSTRAINT fk_audit_actor FOREIGN KEY (actor_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS access_log (                 -- RNF-06: 1 linha por titular exposto
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  occurred_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  user_id       BIGINT UNSIGNED NOT NULL,
  action        VARCHAR(48) NOT NULL,                     -- ninebox.matrix.view, candidate.restricted.view, attachment.download, candidate.export
  resource_type VARCHAR(40) NOT NULL,
  resource_id   BIGINT UNSIGNED NULL,
  subject_type  VARCHAR(16) NOT NULL,
  subject_id    BIGINT UNSIGNED NOT NULL,
  request_id    CHAR(16) NULL,
  ip            VARCHAR(45) NULL,
  PRIMARY KEY (id),
  KEY ix_access_subject (subject_type, subject_id, occurred_at),
  KEY ix_access_user (user_id, occurred_at),
  KEY ix_access_time (occurred_at),
  CONSTRAINT ck_access_subject CHECK (subject_type IN ('candidate','person')),
  CONSTRAINT fk_access_user FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS job_runs (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  job_name     VARCHAR(64) NOT NULL,
  slot_at      DATETIME(3) NOT NULL,                      -- instante agendado (manual = NOW(3))
  origin       VARCHAR(8)  NOT NULL,
  unit         VARCHAR(128) NOT NULL,                     -- hostname da unidade
  status       VARCHAR(8)  NOT NULL,
  started_at   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  finished_at  DATETIME(3) NULL,
  error        TEXT NULL,
  stats        JSON NULL,
  triggered_by BIGINT UNSIGNED NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_job_runs_slot (job_name, slot_at),        -- 1 execução por slot entre N unidades
  KEY ix_job_runs_recent (job_name, started_at),
  CONSTRAINT ck_job_runs_origin CHECK (origin IN ('schedule','manual')),
  CONSTRAINT ck_job_runs_status CHECK (status IN ('running','ok','error')),
  CONSTRAINT fk_job_runs_user FOREIGN KEY (triggered_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS notifications (              -- in-app (RF-REC-10, RF-TRN-06)
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id    BIGINT UNSIGNED NOT NULL,
  kind       VARCHAR(48) NOT NULL,
  title      VARCHAR(200) NOT NULL,
  body       VARCHAR(1000) NULL,
  link_path  VARCHAR(255) NULL,                           -- rota da SPA
  ref_type   VARCHAR(24) NULL,                            -- candidate|training_request|ninebox_entry|quarter|lgpd_request
  ref_id     BIGINT UNSIGNED NULL,
  dedupe_key VARCHAR(191) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  read_at    DATETIME(3) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_notif_dedupe (dedupe_key),
  KEY ix_notif_user (user_id, read_at, created_at),
  KEY ix_notif_ref (ref_type, ref_id),
  CONSTRAINT fk_notif_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_activity_days (         -- métrica "gestores ativos/mês"
  user_id     BIGINT UNSIGNED NOT NULL,
  activity_on DATE NOT NULL,
  PRIMARY KEY (user_id, activity_on),
  CONSTRAINT fk_activity_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

SET FOREIGN_KEY_CHECKS = 1;
