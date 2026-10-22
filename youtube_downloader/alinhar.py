#!/usr/bin/env python3
"""Descobre o deslocamento (em frames) entre dois vídeos do mesmo conteúdo e
monta o comando VMAF já alinhado.

Compara um trecho curto dos dois arquivos com PSNR, testando vários
deslocamentos, e escolhe o que deixa as imagens mais parecidas.

Exemplo:
    python alinhar.py AV1.mp4 AVC.mp4            # testado, referência
    python alinhar.py AV1.mp4 AVC.mp4 --max 10   # procura até ±10 s
"""
import argparse
import re
import shlex
import subprocess
import sys

FPS = 30000 / 1001
FPS_STR = "30000/1001"
TRECHO = 90  # frames comparados em cada teste (~3 s)


def ss(t: float) -> str:
    """Valor de -ss meio frame antes de t: pular exatamente no timestamp de um
    frame faz o ffmpeg descartar esse frame e desalinha a comparação em 1."""
    return f"{max(0.0, t - 0.5 / FPS):.4f}" if t > 0 else "0"


def psnr(testado: str, ref: str, t_testado: float, t_ref: float) -> float:
    """PSNR médio de um trecho, em resolução reduzida para ser rápido."""
    norm = f"fps={FPS_STR},scale=480:270,format=gray,trim=end_frame={TRECHO},setpts=PTS-STARTPTS"
    cmd = [
        "ffmpeg", "-hide_banner", "-nostats",
        "-ss", ss(t_testado), "-i", testado,
        "-ss", ss(t_ref), "-i", ref,
        "-lavfi", f"[0:v]{norm}[a];[1:v]{norm}[b];[a][b]psnr",
        "-an", "-f", "null", "-",
    ]
    saida = subprocess.run(cmd, capture_output=True, text=True).stderr
    m = re.search(r"average:(inf|[\d.]+)", saida)
    if not m:
        return 0.0
    return 100.0 if m.group(1) == "inf" else float(m.group(1))


def melhor(testado, ref, inicio, candidatos):
    resultados = []
    for frames in candidatos:
        d = frames / FPS
        # d > 0: o conteúdo aparece mais tarde no testado
        t_test, t_ref = (inicio + d, inicio) if d >= 0 else (inicio, inicio - d)
        resultados.append((psnr(testado, ref, t_test, t_ref), frames))
    return max(resultados)


def main() -> None:
    p = argparse.ArgumentParser(description="Alinhar dois vídeos para VMAF")
    p.add_argument("testado", help="Arquivo avaliado (ex.: o AV1)")
    p.add_argument("referencia", help="Arquivo de referência (ex.: o AVC ou o master)")
    p.add_argument("--inicio", type=float, default=60,
                   help="Segundo do trecho usado na busca (evite vinhetas/telas pretas). Padrão: 60")
    p.add_argument("--max", type=float, default=5,
                   help="Maior deslocamento procurado, em segundos. Padrão: 5")
    args = p.parse_args()

    limite = round(args.max * FPS)
    print(f"Procurando deslocamento de até ±{args.max:g} s...", file=sys.stderr)
    # Busca grossa (de 10 em 10 frames) e depois fina (frame a frame)
    _, grosso = melhor(args.testado, args.referencia, args.inicio + args.max,
                       range(-limite, limite + 1, 10))
    nota, frames = melhor(args.testado, args.referencia, args.inicio + args.max,
                          range(grosso - 10, grosso + 11))

    print(f"\nDeslocamento: {frames:+d} frames ({frames / FPS:+.3f} s), PSNR do trecho: {nota:.1f} dB")
    if nota < 30:
        print("Atenção: PSNR baixo mesmo no melhor ponto. Os vídeos podem ter edições "
              "diferentes, ou o deslocamento é maior que --max.")

    d = frames / FPS
    ss_test, ss_ref = (d, 0.0) if d >= 0 else (0.0, -d)
    norm = f"setpts=PTS-STARTPTS,scale=1920:1080:flags=bicubic,fps={FPS_STR}"
    filtro = (f"[0:v]{norm}[dist];[1:v]{norm}[ref];"
              f"[dist][ref]libvmaf=log_fmt=json:log_path=vmaf.json:n_threads=8:"
              f"feature=name=psnr|name=float_ssim")
    cmd = ["ffmpeg", "-ss", ss(ss_test), "-i", args.testado,
           "-ss", ss(ss_ref), "-i", args.referencia,
           "-lavfi", filtro, "-shortest", "-an", "-f", "null", "-"]
    print("\nComando VMAF alinhado:\n")
    print(" ".join(shlex.quote(c) for c in cmd))


if __name__ == "__main__":
    main()
