# Baixar vídeos do YouTube

Script em Python sobre o [yt-dlp](https://github.com/yt-dlp/yt-dlp), a ferramenta open source mais usada e mantida para isso.

## Instalação

```bash
pip install -r requirements.txt
```

Instale também o **ffmpeg** (recomendado). Sem ele a qualidade fica limitada (geralmente até 720p) e não dá para extrair MP3.
- Windows: `winget install ffmpeg`
- macOS: `brew install ffmpeg`
- Linux: `sudo apt install ffmpeg`

### macOS

O Python que vem no macOS é o 3.9, e o yt-dlp atual exige 3.10+. Com o 3.9 o pip instala uma versão antiga, que o YouTube já bloqueia ("The page needs to be reloaded"). Use o Python do Homebrew:

```bash
brew install python@3.13 ffmpeg deno
python3.13 -m venv ~/yt-env
source ~/yt-env/bin/activate
pip install -r requirements.txt
```

O `deno` é o runtime JavaScript que o yt-dlp usa para resolver os desafios do YouTube.

## Uso

```bash
# Melhor qualidade disponível, salvo em ./downloads como MP4
python baixar.py "https://www.youtube.com/watch?v=o7VtQT4g_HI"

# Limitar a 720p
python baixar.py "URL" --qualidade 720

# MP4 em H.264: abre em qualquer player, TV ou editor (máx. 1080p)
python baixar.py "URL" --compativel

# Priorizar AV1 (arquivo menor; só se o YouTube tiver AV1 para esse vídeo)
python baixar.py "URL" --av1

# Só o áudio, em MP3
python baixar.py "URL" --audio

# Ver título, duração e resoluções sem baixar
python baixar.py "URL" --info

# Outras opções: --saida PASTA, --legendas, --playlist
python baixar.py --help
```

Coloque o link entre aspas: links com `&` quebram no terminal sem elas.

## Problemas comuns

- **Erro "Sign in to confirm you're not a bot" ou HTTP 403**: o YouTube muda com frequência. Rode `pip install -U yt-dlp`.
- **Vídeo com restrição de idade ou privado**: use cookies do navegador com o yt-dlp direto: `yt-dlp --cookies-from-browser chrome "URL"`.

## Uso responsável

Os Termos de Serviço do YouTube proíbem baixar conteúdo, exceto quando o próprio YouTube oferece essa opção ou o dono dos direitos autoriza. Use para seus próprios vídeos, conteúdo com licença Creative Commons/domínio público ou com autorização.
