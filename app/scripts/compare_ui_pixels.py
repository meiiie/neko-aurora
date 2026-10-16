import argparse
import hashlib
import json
import pathlib

from PIL import Image, ImageChops, ImageStat


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--reference", required=True, type=pathlib.Path)
    parser.add_argument("--evidence", required=True, type=pathlib.Path)
    args = parser.parse_args()
    files = {
        "reference": args.reference,
        "before": args.evidence / "before-f6-desktop-1280x720.png",
        "after": args.evidence / "after-light-desktop-1280x720.png",
    }
    images = {name: Image.open(file).convert("RGB") for name, file in files.items()}
    assert all(image.size == (1280, 720) for image in images.values())
    samples = {"main": (1100, 620), "sidebar": (150, 400), "user": (1200, 140)}
    report = {
        "viewport": [1280, 720],
        "scope": "RGB comparison only; actors, fixture state and selected conversation differ. This is not UX acceptance.",
        "imageSha256": {name: hashlib.sha256(file.read_bytes()).hexdigest() for name, file in files.items()},
        "pixelSamples": {name: {key: image.getpixel(point) for key, point in samples.items()} for name, image in images.items()},
        "meanAbsoluteRGBError": {
            name: sum(ImageStat.Stat(ImageChops.difference(images["reference"], images[name])).mean) / 3
            for name in ("before", "after")
        },
    }
    ImageChops.difference(images["reference"], images["after"]).save(args.evidence / "reference-vs-light-rgb-difference.png")
    pair = Image.new("RGB", (2560, 720))
    pair.paste(images["reference"], (0, 0))
    pair.paste(images["after"], (1280, 0))
    pair.save(args.evidence / "reference-and-light-1280x720.png")
    (args.evidence / "comparison.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
