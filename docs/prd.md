# PRD — Plataforma de Gestão do Ciclo de Vida do Colaborador

| Campo | Valor |
|---|---|
| Status | **Rascunho v0.1 — em revisão** |
| Data | 2026-10-17 |
| Autor / Product Owner | Marcos Coelho |
| Próximos artefatos | `docs/sdd.md` (Software Design Doc) → plano de implementação |

> Convenção de IDs: `RF-<MÓDULO>-NN` = requisito funcional; `RNF-NN` = requisito não funcional. Prioridade: **M** (Must / MVP), **S** (Should / MVP se couber), **C** (Could / pós-MVP).

---

## 1. Resumo e problema

### 1.1 Contexto
A gerência tem vários times de desenvolvimento e não tem processos nem ferramentas para gerir o **ciclo de vida do colaborador**: atrair e entrevistar candidatos, receber novas pessoas, acompanhar comportamento e performance e investir em capacitação.

### 1.2 Dores atuais
| # | Dor | Consequência |
|---|---|---|
| D1 | Cada gestor mantém o **próprio banco de candidatos** (planilhas, e-mail, chat). | Retrabalho. Bons candidatos indicados ou já entrevistados se perdem. Não há visão compartilhada nem assíncrona. |
| D2 | O onboarding é **artesanal** e depende da memória do gestor. | A experiência varia entre times, conteúdos comuns são recriados e o time-to-productivity é maior. |
| D3 | A avaliação de comportamento e performance **não tem registro histórico estruturado**. | Calibração subjetiva, perda de contexto quando o gestor muda e falta de base para promoções e PDIs. |
| D4 | Pleitos de treinamento chegam ao Head **de forma dispersa**. | Não há visão de custo por quarter, a aprovação não é rastreável e não se sabe o que foi pago ou concluído. |

### 1.3 Proposta
Uma **plataforma web interna, restrita a gestores**, com quatro módulos integrados que cobrem o ciclo **Recrutamento → Onboarding → Gestão contínua (Nine Box) → Desenvolvimento (Treinamentos)**. A plataforma **consolida e complementa o ERP da Globo**, sem substituí-lo.

```
 Candidato ──► [1. Recrutamento] ──contratado──► [2. Onboarding] ──► Colaborador ativo
                                                                          │
                                       ┌──────────────────────────────────┤
                                       ▼                                  ▼
                              [3. Nine Box por quarter]      [4. Pleitos de treinamento]
                                       ▲                                  │
                                       └──── insumo para PDI / pleitos ◄──┘
```

---

## 2. Objetivos e não-objetivos

### 2.1 Objetivos
- **O1 — Consolidar o ERP, não substituí-lo.** A plataforma é a **camada única de gestão** usada **em conjunto com o ERP**. O ERP continua sendo a fonte oficial dos dados cadastrais (colaborador, cargo, hierarquia, time, centro de custo). A plataforma consome esses dados e os **enriquece** com o que o ERP não cobre: banco de candidatos, trilhas de onboarding, Nine Box e pleitos de treinamento.
- **O2** — Criar um **banco de candidatos compartilhado** entre os gestores da gerência.
- **O3** — **Padronizar o onboarding** com trilhas comuns da gerência e trilhas específicas por time.
- **O4** — Ter um **registro histórico, quarter a quarter**, de comportamento × performance de cada dev, com calibração entre pares.
- **O5** — Dar ao Head uma **visão trimestral consolidada** dos pleitos de treinamento, com aprovação rastreável e acompanhamento de pagamento e conclusão.
- **O6** — Garantir **acesso exclusivo de gestores** via SSO corporativo e grupos.

### 2.2 Não-objetivos (fora de escopo)
- Substituir o ERP ou a folha, ou ser o sistema oficial de RH. **Escrita no ERP fica fora do MVP.**
- Dar acesso direto ao colaborador ou ao candidato (o onboardee recebe material compartilhado, ver Módulo 2).
- Fazer avaliação 360° completa, autoavaliação ou cálculo de remuneração ou bônus.
- Processar pagamentos de treinamento. A plataforma **registra o status** do pagamento, não paga.
- Substituir o ATS oficial (Gupy). A plataforma **integra** com ele.

---

## 3. Personas e papéis (RBAC)

| Papel | Quem | Pode |
|---|---|---|
| **Gestor** | Gestores dos times da gerência (grupo de gestores no Backstage "Minha Equipe"). | CRUD de candidatos e entrevistas. Gerar e acompanhar onboarding dos próprios liderados. Registrar Nine Box dos liderados diretos. Comentar na calibração dos pares. Criar e submeter pleitos de treinamento do próprio time. |
| **Head** | Head da gerência. | Tudo que o Gestor pode, mais: ver todos os Nine Box, abrir e fechar a calibração do quarter, aprovar ou reprovar pleitos em lote, definir o orçamento do quarter e ver dados restritos de candidatos. |
| **Admin** | 1 ou 2 pessoas indicadas (pode ser o próprio Head). | Gerir catálogos (trilhas de onboarding, stacks, cargos, competências), quarters, configurações e feature flags. Executar sincronização com ERP/Gupy. |

- **RF-AUTH-01 (M)** — O papel é derivado de **claims de grupo do token OIDC**. O mapeamento grupo → papel é configurável.
- **RF-AUTH-02 (M)** — Usuário autenticado **sem** grupo de gestor recebe "acesso negado" e nenhum dado é exposto.
- **RF-AUTH-03 (S)** — O vínculo gestor ↔ liderados vem do ERP (hierarquia) e o Admin pode sobrescrever manualmente em caso de exceção, com auditoria.

---

## 4. Módulo 1 — Recrutamento (banco de candidatos)

**Objetivo:** qualquer gestor da gerência consegue registrar, consultar e reaproveitar candidatos indicados ou entrevistados por outros gestores, de forma **assíncrona**.

### 4.1 Ficha do candidato
| Grupo | Campos | Prioridade |
|---|---|---|
| **Identificação e contato** | Nome; e-mail; telefone; LinkedIn; GitHub/portfólio; cidade/UF; modelo de trabalho aceito (remoto/híbrido/presencial); disponibilidade para início; aviso prévio. | M |
| **Perfil profissional** | Cargo pretendido (catálogo vindo do ERP); senioridade (Jr / Pl / Sr / Especialista / Staff); perfil (Backend, Frontend, Mobile, Dados, SRE/Plataforma, QA, Segurança, Gestão…); área/time de interesse; anos de experiência; inglês e outros idiomas (nível); se é ex-colaborador Globo; empresa e cargo atuais. | M |
| **Conhecimento de stack** | Lista estruturada de **tecnologia + nível (1–5) + anos de uso**, a partir de um **catálogo controlado** (linguagens, frameworks, cloud, bancos, mensageria, observabilidade, IaC), mais **tags livres** complementares. | M |
| **Origem** | Indicação (quem indicou: colaborador ou gestor), entrevista direta, Gupy/ATS, evento ou comunidade, hunting. Vaga(s) relacionada(s). | M |
| **Anexos** | CV (PDF/DOCX), portfólio, teste técnico. Limite de tamanho e tipos permitidos. | M |
| **Dados restritos** | Pretensão salarial; motivo de reprovação; observações sensíveis. **Visíveis só para quem registrou e para o Head.** | S |
| **LGPD** | Base legal e consentimento; data de coleta; **data de expiração do registro**; solicitação de exclusão. | M |

> Dados pessoais sensíveis (saúde, PCD, etnia, religião etc. — art. 5º, II da LGPD) **não são coletados no MVP**. Se houver demanda de diversidade e inclusão no futuro, ela entra com base legal própria e aval do Jurídico.

### 4.2 Requisitos funcionais
- **RF-REC-01 (M)** — Cadastrar, editar e arquivar candidatos com a ficha acima.
- **RF-REC-02 (M)** — **Deduplicação**: alertar no cadastro quando já existir um candidato com o mesmo e-mail ou LinkedIn e oferecer a opção de abrir o registro existente.
- **RF-REC-03 (M)** — **Registro de entrevistas** (várias por candidato, por gestores diferentes): entrevistador, data, tipo (técnica, comportamental, fit cultural, case), **scorecard por competência** (escala 1–5, competências vindas do catálogo), recomendação (Forte sim / Sim / Não / Forte não) e parecer em texto.
- **RF-REC-04 (M)** — **Status no funil**: Banco de talentos → Em processo → Aprovado → Contratado. Status terminais: Reprovado e Desistiu. Cada transição é registrada com autor e data.
- **RF-REC-05 (M)** — **Busca e filtros**: texto livre, stack + nível mínimo (ex.: "Go ≥ 4"), senioridade, perfil, cargo, status, origem, tags e período.
- **RF-REC-06 (M)** — Timeline do candidato: todas as interações, entrevistas e mudanças de status, com autor.
- **RF-REC-07 (M)** — Visibilidade: o banco é **visível a todos os gestores da gerência**, exceto os dados restritos (4.1).
- **RF-REC-08 (S)** — Candidato com status **Contratado** gera um **pré-cadastro de onboarding** (Módulo 2), com nome, cargo e time de destino.
- **RF-REC-09 (S)** — **Integração com ATS (Gupy)**: importar candidatos e vagas com mapeamento de campos e sem duplicar (usa RF-REC-02). Fica atrás de feature flag, porque depende de API e contrato.
- **RF-REC-10 (M)** — **Retenção**: aviso ao responsável antes da expiração e anonimização automática dos registros vencidos sem renovação de consentimento.
- **RF-REC-11 (C)** — Comparar até 3 candidatos lado a lado (scorecards).

---

## 5. Módulo 2 — Onboarding

**Objetivo:** montar o material de onboarding de uma nova pessoa combinando a **trilha comum da gerência** com a **trilha do time** de destino.

### 5.1 Conceitos
- **Catálogo de aprendizado**: submódulos **comuns** (cultura, ferramentas corporativas, processos da gerência, segurança, arquitetura geral) e submódulos **por time** (domínio, repositórios, rituais, on-call, stack).
- **Item de trilha**: título, descrição, tipo (leitura, vídeo, reunião, tarefa prática, acesso a solicitar), link, **responsável/buddy**, **prazo relativo** (D+1, D+7, D+30, D+90) e se é obrigatório.
- **Plano de onboarding**: instância gerada para uma pessoa = trilha comum + trilha(s) do time selecionado + itens avulsos.

### 5.2 Requisitos funcionais
- **RF-ONB-01 (M)** — Admin e gestores mantêm o catálogo: submódulos comuns e por time, com itens, versionamento e ordenação.
- **RF-ONB-02 (M)** — Gerar um plano informando nome, cargo, **time de destino** e data de início. O sistema compõe a trilha comum com a trilha do time e calcula as datas a partir dos prazos relativos.
- **RF-ONB-03 (M)** — O gestor pode adicionar, remover ou ajustar itens no plano gerado sem alterar o catálogo.
- **RF-ONB-04 (M)** — **Checklist de acompanhamento** pelo gestor: status por item (pendente, em andamento, concluído, não se aplica) e % de progresso.
- **RF-ONB-05 (M)** — **Compartilhamento com o novo colaborador**, que **não faz login**: link de leitura com token não adivinhável, com expiração e revogável, e/ou exportação em PDF/Markdown.
- **RF-ONB-06 (S)** — Painel do gestor e do Head: onboardings em andamento, itens atrasados e tempo médio de conclusão.
- **RF-ONB-07 (S)** — Plano criado a partir de candidato contratado (RF-REC-08).
- **RF-ONB-08 (C)** — Checkpoints de feedback 30/60/90 dias registrados pelo gestor.

---

## 6. Módulo 3 — Nine Box (comportamento × performance)

**Objetivo:** registrar e consultar, **quarter a quarter**, a posição de cada dev numa matriz 3×3 e manter o histórico de evolução.

### 6.1 Modelo
- **Eixos (MVP):** **Performance** (entregas, qualidade, impacto) × **Comportamento** (valores, colaboração, postura). Escala Baixo / Médio / Alto em cada eixo, totalizando 9 quadrantes nomeados (ex.: "Destaque", "Consistente", "Atenção").
- Os eixos, os rótulos e os critérios são **configuráveis pelo Admin**, para permitir adotar no futuro o formato clássico performance × potencial.

### 6.2 Fluxo por quarter
```
Quarter aberto ─► Gestor registra (rascunho) ─► Submete ─► Calibração (pares comentam, Head media)
               ─► Head fecha o quarter ─► Registro IMUTÁVEL (só retificação auditada)
```

### 6.3 Requisitos funcionais
- **RF-9BOX-01 (M)** — Admin ou Head cria quarters (ex.: 2026-Q4) com datas de abertura, calibração e fechamento.
- **RF-9BOX-02 (M)** — O **gestor direto** registra, por liderado e por quarter: nota em cada eixo, quadrante resultante, **justificativa obrigatória** com evidências, pontos fortes, pontos de desenvolvimento e ações combinadas.
- **RF-9BOX-03 (M)** — **Calibração**: os **pares** (gestores participantes da calibração) veem os registros submetidos e podem **comentar**. O gestor pode ajustar até o fechamento. Os ajustes ficam registrados (antes → depois, autor, motivo).
- **RF-9BOX-04 (M)** — O **Head fecha o quarter** e os registros ficam somente leitura. Retificação posterior só pelo Head, com motivo e auditoria.
- **RF-9BOX-05 (M)** — **Histórico quarter a quarter** por dev: linha do tempo dos quadrantes, justificativas e comentários.
- **RF-9BOX-06 (M)** — **Visão da matriz** por quarter, filtrável por time e por gestor, com a distribuição dos devs nos quadrantes.
- **RF-9BOX-07 (S)** — **Movimentação**: quem subiu, desceu ou se manteve em relação ao quarter anterior.
- **RF-9BOX-08 (M)** — Visibilidade: o registro de um dev é visível **somente** ao gestor direto, aos pares **durante e após a calibração** daquele quarter e ao Head.
- **RF-9BOX-09 (S)** — Indicador de cobertura: % de liderados com registro no quarter e pendências por gestor.
- **RF-9BOX-10 (C)** — Link de um registro de Nine Box com um pleito de treinamento (ação de desenvolvimento).

---

## 7. Módulo 4 — Pleitos e acompanhamento de treinamentos

**Objetivo:** o gestor registra os pleitos de treinamento do time e o Head os avalia trimestralmente, com base no orçamento. Depois a plataforma acompanha pagamento e conclusão.

### 7.1 Ficha do pleito
Nome do treinamento; fornecedor/instituição; modalidade (online, presencial, conferência, certificação); link; **conteúdo abordado**; carga horária; datas previstas; **dev(s) beneficiado(s)**; **custo** (inscrição, viagem, hospedagem e outros, mais o total); justificativa e ganho esperado; prioridade definida pelo gestor; **quarter de referência**; anexos (proposta, orçamento, nota fiscal, certificado).

### 7.2 Ciclo de vida (check-in / check-out)
```
Rascunho ─► Submetido ─► Aprovado ─► Pago ─► Em andamento ─► Concluído
                     └─► Reprovado            └─► Cancelado (com motivo, a partir de qualquer estado pós-aprovação)
```

### 7.3 Requisitos funcionais
- **RF-TRN-01 (M)** — O gestor cria, edita e submete pleitos para o quarter vigente ou o próximo.
- **RF-TRN-02 (M)** — Head ou Admin define o **orçamento por quarter** (por gerência; por time é ponto em aberto, §12).
- **RF-TRN-03 (M)** — **Painel trimestral do Head**: pleitos submetidos agrupados por time, custo total vs. orçamento, saldo e prioridade. Aprovação ou reprovação **em lote ou individual**, com comentário obrigatório na reprovação.
- **RF-TRN-04 (M)** — **Check-in e check-out de status**: cada transição (aprovado, pago, iniciado, concluído) registra autor, data e evidência (ex.: comprovante de pagamento, certificado).
- **RF-TRN-05 (M)** — Histórico de pleitos por dev e por time, com filtros de quarter e status.
- **RF-TRN-06 (S)** — Notificação ao gestor quando o Head decide sobre o pleito, e ao Head quando a janela do quarter fecha.
- **RF-TRN-07 (S)** — Exportar o consolidado do quarter (CSV/PDF) para enviar a Finanças/RH.
- **RF-TRN-08 (C)** — Avaliação pós-treinamento pelo gestor (o investimento valeu?).

---

## 8. Requisitos não funcionais

| ID | Requisito | Prior. |
|---|---|---|
| RNF-01 | **SSO corporativo Globo via OIDC**: login único e sessão segura. Sem senhas locais. | M |
| RNF-02 | **Autorização por grupos**: claims de grupo do token. O grupo de gestores é gerido no **Backstage "Minha Equipe"**. Checagem no backend em todo endpoint (não só na UI). | M |
| RNF-03 | **Deploy no Tsuru**. Configuração por variáveis de ambiente, sem segredos no repositório. | M |
| RNF-04 | **Banco gerenciado no DBaaS** (MySQL, preferência atual). Backups e migrations versionadas. | M |
| RNF-05 | **Armazenamento de arquivos** (CVs, comprovantes) em object storage interno, a confirmar. Nada de arquivos em disco local do container. | M |
| RNF-06 | **LGPD**: minimização, base legal por tipo de dado, retenção e anonimização de candidatos, atendimento a pedido de exclusão e **log de acesso a dados sensíveis** (Nine Box, dados restritos de candidato). | M |
| RNF-07 | **Auditoria**: todas as escritas em Nine Box, status de pleitos e funil de candidatos registram quem, quando, o quê e o valor anterior. | M |
| RNF-08 | **Segurança**: OWASP Top 10, validação de upload (tipo, tamanho e varredura de malware, se houver serviço), links de onboarding com token aleatório e expiração. | M |
| RNF-09 | **Observabilidade**: logs estruturados, métricas e health-check compatíveis com o padrão Tsuru/Globo. | S |
| RNF-10 | **Disponibilidade**: horário comercial é suficiente. Não é crítico 24×7. Meta inicial de 99%. | S |
| RNF-11 | **Desempenho**: busca no banco de candidatos em < 1s para até ~10 mil registros. | S |
| RNF-12 | **Usabilidade**: interface em pt-BR, responsiva (desktop prioritário) e acessível (WCAG AA básico). | S |
| RNF-13 | **Integrações desacopladas** (ERP, Gupy): falha do sistema externo não derruba a plataforma e a sincronização pode ser reprocessada. | M |

**Stack preferida (a validar no SDD):** backend em **Go**, frontend em **React**, banco **MySQL** (DBaaS), deploy no **Tsuru**.

---

## 9. Métricas de sucesso

| Métrica | Meta inicial (6 meses após o go-live) |
|---|---|
| Gestores ativos/mês ÷ gestores da gerência | ≥ 80% |
| Candidatos consultados ou reaproveitados por um gestor diferente de quem cadastrou | ≥ 20% dos candidatos em processo |
| Tempo para montar um plano de onboarding | < 15 min (hoje: a medir) |
| Devs com Nine Box registrado no quarter | 100% no fechamento |
| Lead time submissão → decisão do pleito | ≤ 10 dias úteis após o fim da janela |
| Pleitos com status final rastreado (pago/concluído) | ≥ 90% |

> Linha de base: medir antes do go-live (questionário rápido com os gestores) para que as metas não sejam arbitrárias.

---

## 10. Fases e escopo

| Fase | Conteúdo |
|---|---|
| **MVP — Fundação** | SSO + RBAC por grupos, leitura de colaboradores/hierarquia do ERP (ou carga por arquivo como contingência), catálogos básicos, auditoria. |
| **MVP — Módulos** | Requisitos **M** dos 4 módulos. Ordem sugerida: Recrutamento → Treinamentos → Nine Box (alinhada ao primeiro quarter de uso) → Onboarding. |
| **MVP — Final** | Integração Gupy (RF-REC-09) atrás de feature flag e requisitos **S** priorizados. |
| **Pós-MVP** | Requisitos **C**, escrita/sincronização bidirecional com o ERP, notificações por Slack/e-mail, analytics. |

> A ordem dos módulos é uma recomendação para revisarmos: Recrutamento entrega valor imediato (dor D1). O Nine Box depende do calendário do quarter.

---

## 11. Riscos e premissas

| # | Risco / premissa | Mitigação |
|---|---|---|
| R1 | **"Shadow HR"**: a plataforma vira um sistema de RH paralelo e entra em conflito com o ERP e o RH oficial. | ERP como fonte de verdade (O1). Alinhamento com o RH desde o PRD. Escopo explícito de "apoio à gestão". |
| R2 | **LGPD** em dados de candidatos e avaliações. | Revisão de RH e Jurídico antes do go-live. Minimização, retenção e logs (RNF-06). |
| R3 | **Dependência de integrações** (IdP/grupos, ERP, Gupy, object storage) sem API ou acesso liberado. | Planos de contingência: carga por arquivo (ERP) e cadastro manual (Gupy). Feature flags. |
| R4 | **Adoção**: gestores continuam nas planilhas. | Migração inicial de dados existentes, UX simples e patrocínio do Head (ex.: pleitos de treinamento só por esta via). |
| R5 | **Viés no Nine Box** (efeito halo, recência, leniência). | Justificativa obrigatória com evidências, calibração entre pares e critérios descritos por quadrante. |
| R6 | Conflito de visibilidade (ex.: gestor vê o Nine Box de quem não lidera). | Regras de RF-9BOX-08 aplicadas no backend e cobertas por testes. |
| P1 | Premissa: as claims de grupo já chegam no token OIDC. | Confirmar com o time de identidade (§12). |

---

## 12. Perguntas em aberto

| # | Pergunta | Dono | Impacto |
|---|---|---|---|
| Q1 | **Qual é o ERP de pessoas** (SAP SuccessFactors? Senior? outro)? Ele tem **API** de leitura de colaboradores, cargos e hierarquia ou só exportação? Qual frequência de sincronização é aceitável? | Marcos / RH | Arquitetura de integração (O1) |
| Q2 | Qual é o **IdP** (Keycloak? Azure AD?), o **nome da claim** e o **nome do grupo**? Como o grupo do Backstage "Minha Equipe" chega ao token? | Time de identidade / Backstage | RNF-01/02 |
| Q3 | Quem são os **"pares" na calibração**: todos os gestores da gerência ou só os da mesma coordenação? | Head | RF-9BOX-03/08 |
| Q4 | **Gupy**: há API e contrato liberados? É importação pontual ou sincronização contínua? | RH / Recrutamento | RF-REC-09 |
| Q5 | Qual **object storage** está disponível no ecossistema Tsuru/Globo para anexos? | Plataforma | RNF-05 |
| Q6 | O **orçamento de treinamento** é definido por gerência ou por time? | Head | RF-TRN-02/03 |
| Q7 | Qual é o **prazo de retenção** de candidatos aceito pelo Jurídico (ex.: 12 ou 24 meses)? | Jurídico | RF-REC-10 |
| Q8 | Os nomes dos quadrantes e os critérios dos eixos já existem na gerência ou precisam ser definidos? | Head / gestores | RF-9BOX-02 |
| Q9 | Existe a figura de **buddy/padrinho** no onboarding hoje? | Gestores | RF-ONB-01 |

---

## 13. Referências de mercado (e contrapontos)

- **Nine Box**: nasceu da matriz GE/McKinsey (anos 1970) para portfólios de negócio e foi adaptada para pessoas. O formato clássico é **performance × potencial**.
  - *A favor da variação "comportamento × performance"*: evita o rótulo "potencial", que é criticado por ser subjetivo e enviesado. Ver as críticas de **Marcus Buckingham & Ashley Goodall** (*Nine Lies About Work*, 2019, sobre o "idiosyncratic rater effect") e de **Josh Bersin** (sobre a calibração forçada).
  - *Contra*: perde-se a leitura de sucessão e de potencial de crescimento. Por isso os eixos são **configuráveis** (§6.1).
- **Performance e calibração**: Lattice, Culture Amp e 15Five, que trazem ciclos, calibração entre gestores e histórico. São referência de UX, mas com custo de licença e dados fora da Globo, e não resolvem a integração com o Backstage.
- **Recrutamento**: Gupy (ATS dominante no Brasil) e Greenhouse (popularizou o *structured interviewing* com scorecards por competência). A pesquisa de **Schmidt & Hunter (1998)** e a atualização de **Sackett et al. (2022)** indicam que entrevistas estruturadas estão entre os melhores preditores de desempenho. Isso justifica o scorecard em RF-REC-03.
- **Onboarding**: Rippling, BambooHR e o modelo de trilhas 30/60/90. A pesquisa de **Talya Bauer (SHRM Foundation, "Onboarding New Employees")** organiza o onboarding nos "4 Cs": Compliance, Clarification, Culture, Connection. A trilha comum cobre Compliance e Culture, e a trilha do time cobre Clarification e Connection.
- **Build vs. buy**: comprar uma suíte (ex.: Lattice) resolveria parte dos módulos 1 e 3, mas não a integração com Backstage/Tsuru/ERP nem os fluxos específicos de pleito. Construir internamente é mais aderente, mas cria custo de manutenção. **Recomendação:** construir, com escopo enxuto (MVP) e revisando a decisão após 2 quarters de uso.

---

## 14. Rastreabilidade — pedido original → PRD

| # | Pedido original | Onde está no PRD |
|---|---|---|
| 1 | Plataforma para o gestor conduzir, visualizar e gerenciar todo o ciclo de vida do funcionário | §1.3, §2.1 |
| 2 | Consolidar o ERP, não substituí-lo, e ser usada em conjunto | O1, §2.2, R1, Q1 |
| 3 | Módulo de recrutamento: registrar, visualizar e consultar candidatos indicados ou entrevistados | RF-REC-01, 03, 05, 06 |
| 4 | Banco compartilhado entre gestores, de forma assíncrona | RF-REC-07, O2 |
| 5 | Ficha com perfil, cargo, conhecimento de stack e outras informações relevantes | §4.1 |
| 6 | CV/anexos, histórico de entrevistas, busca por skills/tags, integração com ATS | §4.1, RF-REC-03, 05, 09 |
| 7 | Onboarding com submódulos comuns da gerência e específicos por time | §5.1, RF-ONB-01 |
| 8 | Gerar material de onboarding conforme a área de destino | RF-ONB-02, 05 |
| 9 | Nine Box de comportamento e performance registrado por gestores e pares | §6.1, RF-9BOX-02, 03 |
| 10 | Registrar e consultar quarter a quarter, com histórico | RF-9BOX-01, 04, 05 |
| 11 | Pleito de treinamento com informações básicas, custos, conteúdo e dev beneficiado | §7.1, RF-TRN-01 |
| 12 | Apresentação trimestral ao Head para aprovação | RF-TRN-02, 03 |
| 13 | Check-in/check-out: aprovado, pago, etc. | §7.2, RF-TRN-04 |
| 14 | Ambiente restrito a gestores | §3, RF-AUTH-01/02, RNF-02 |
| 15 | SSO com o ambiente interno da Globo | RNF-01 |
| 16 | Política de grupos conversando com o Backstage "Minha Equipe" | RNF-02, Q2 |
| 17 | Deploy no Tsuru | RNF-03 |
| 18 | Banco de dados no DBaaS | RNF-04 |
