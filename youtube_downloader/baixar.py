#!/usr/bin/env python3
"""Baixa vídeos (ou só o áudio) do YouTube usando yt-dlp.

Exemplos:
    python baixar.py https://www.youtube.com/watch?v=o7VtQT4g_HI
    python baixar.py URL --qualidade 720
    python baixar.py URL --audio
    python baixar.py URL --info
"""
import argparse
import shutil
import sys

try:
    from yt_dlp import YoutubeDL
    from yt_dlp.utils import DownloadError
except ImportError:
    sys.exit("yt-dlp não instalado. Rode: pip install -r requirements.txt")


def formato(qualidade: str, tem_ffmpeg: bool) -> str:
    limite = "" if qualidade == "max" else f"[height<={qualidade}]"
    if tem_ffmpeg:
        # Melhor vídeo + melhor áudio separados, unidos pelo ffmpeg
        return f"bv*{limite}[ext=mp4]+ba[ext=m4a]/bv*{limite}+ba/b{limite}"
    # Sem ffmpeg só dá para baixar formatos que já vêm com áudio+vídeo juntos
    return f"b{limite}[ext=mp4]/b{limite}"


def main() -> None:
    p = argparse.ArgumentParser(description="Baixar vídeos do YouTube")
    p.add_argument("urls", nargs="+", help="Um ou mais links (vídeo ou playlist)")
    p.add_argument("-q", "--qualidade", default="max",
                   choices=["max", "2160", "1440", "1080", "720", "480", "360"],
                   help="Altura máxima do vídeo (padrão: max)")
    p.add_argument("-a", "--audio", action="store_true", help="Baixar só o áudio em MP3")
    p.add_argument("-o", "--saida", default="downloads", help="Pasta de destino")
    p.add_argument("--legendas", action="store_true", help="Baixar legendas (pt/en)")
    p.add_argument("--playlist", action="store_true",
                   help="Se o link tiver &list=, baixar a playlist inteira")
    p.add_argument("--info", action="store_true", help="Só mostrar informações, sem baixar")
    args = p.parse_args()

    tem_ffmpeg = shutil.which("ffmpeg") is not None
    if not tem_ffmpeg:
        print("Aviso: ffmpeg não encontrado; qualidade limitada (geralmente até 720p) "
              "e conversão para MP3 indisponível.", file=sys.stderr)
        if args.audio:
            sys.exit("Instale o ffmpeg para usar --audio.")

    opts = {
        "outtmpl": f"{args.saida}/%(title)s [%(id)s].%(ext)s",
        "noplaylist": not args.playlist,
        "restrictfilenames": False,
        "windowsfilenames": True,
    }

    if args.audio:
        opts["format"] = "ba/b"
        opts["postprocessors"] = [{
            "key": "FFmpegExtractAudio",
            "preferredcodec": "mp3",
            "preferredquality": "192",
        }]
    else:
        opts["format"] = formato(args.qualidade, tem_ffmpeg)
        if tem_ffmpeg:
            opts["merge_output_format"] = "mp4"

    if args.legendas:
        opts.update({
            "writesubtitles": True,
            "writeautomaticsub": True,
            "subtitleslangs": ["pt.*", "en.*"],
        })

    try:
        with YoutubeDL(opts) as ydl:
            if args.info:
                for url in args.urls:
                    info = ydl.extract_info(url, download=False)
                    print(f"Título:   {info.get('title')}")
                    print(f"Canal:    {info.get('uploader')}")
                    print(f"Duração:  {info.get('duration_string')}")
                    alturas = sorted({f['height'] for f in info.get('formats', []) if f.get('height')})
                    print(f"Resoluções disponíveis: {', '.join(f'{h}p' for h in alturas)}")
                return
            sys.exit(ydl.download(args.urls))
    except DownloadError:
        # yt-dlp já imprimiu o motivo; dica mais comum de correção:
        sys.exit("Falhou. Tente atualizar: pip install -U yt-dlp")


if __name__ == "__main__":
    main()
