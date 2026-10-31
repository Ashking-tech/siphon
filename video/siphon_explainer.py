"""Siphon — a six-second explainer for the tail-first relocate engine.

The story, in one line:
    remux one keyframe-aligned chunk into the MP4's final offset, verify it,
    then truncate that chunk off the tail of the source.
Only one chunk is ever duplicated, so peak space is source + one chunk.

Render (Manim Community):
    manim -qh --fps 30 siphon_explainer.py SiphonExplainer

No LaTeX is used, so dvisvgm is not required.
"""

import numpy as np
from manim import *

# --------------------------------------------------------------------------- #
# Palette / layout
# --------------------------------------------------------------------------- #

BG = "#0A0F14"
TXT = "#E9F0F6"
MUTED = "#8EA0B2"
DIM = "#4A5B6C"
LINE_C = "#22303D"
SLOT_C = "#355066"
GRID_C = "#1A2530"

SRC_C = "#C98A43"
SRC_HI = "#F0BC72"
SRC_EDGE = "#F6D6A6"
SRC_INK = "#2A1804"

OUT_C = "#42D9BC"
OUT_EDGE = "#A7F7E8"
OUT_INK = "#032B25"

SANS = "Inter"
MONO = "Hack"

BAR_LEFT = -4.55
BAR_RIGHT = 6.15
BAR_W = BAR_RIGHT - BAR_LEFT
BAR_H = 0.64
N_CHUNKS = 5
CHUNK_GAP = 0.06
CHUNK_W = (BAR_W - (N_CHUNKS - 1) * CHUNK_GAP) / N_CHUNKS
SRC_Y = 1.20
OUT_Y = -0.62

CHUNK_CENTERS = [
    BAR_LEFT + CHUNK_W / 2 + i * (CHUNK_W + CHUNK_GAP)
    for i in range(N_CHUNKS)
]


def make_chunk(index: int, center_y: float, source: bool) -> tuple[VGroup, RoundedRectangle]:
    """One visually distinct media chunk. Returns (group, rectangle)."""
    rect = RoundedRectangle(
        width=CHUNK_W,
        height=BAR_H,
        corner_radius=0.075,
        stroke_color=SRC_EDGE if source else OUT_EDGE,
        stroke_width=1.2,
        fill_color=SRC_C if source else OUT_C,
        fill_opacity=0.92,
    )
    rect.move_to([CHUNK_CENTERS[index], center_y, 0])

    number = Text(
        str(index),
        font=MONO,
        font_size=24,
        weight=BOLD,
        color=SRC_INK if source else OUT_INK,
    ).move_to(rect)

    group = VGroup(rect, number)

    if source:
        # A small keyframe pip at the left edge of each source segment.
        pip = Dot(radius=0.024, color="#FFE9C4", fill_opacity=0.9, stroke_width=0)
        pip.move_to(rect.get_left() + RIGHT * 0.08)
        group.add(pip)

    return group, rect


def make_slot(index: int) -> DashedVMobject:
    """The sparse reservation of one output chunk."""
    rect = RoundedRectangle(
        width=CHUNK_W,
        height=BAR_H,
        corner_radius=0.075,
        stroke_color=SLOT_C,
        stroke_width=1.4,
        fill_opacity=0,
    )
    rect.move_to([CHUNK_CENTERS[index], OUT_Y, 0])
    slot = DashedVMobject(rect, num_dashes=18, dash_ratio=0.55)
    slot.set_stroke(SLOT_C, width=1.4)
    return slot


def make_pill(label: str, color: str = MUTED, border: str = LINE_C, fill: float = 0.04) -> VGroup:
    text = Text(label, font=MONO, font_size=17, color=color)
    box = RoundedRectangle(
        width=text.width + 0.34,
        height=0.40,
        corner_radius=0.20,
        stroke_color=border,
        stroke_width=1.2,
        fill_color=border,
        fill_opacity=fill,
    )
    text.move_to(box)
    return VGroup(box, text)


def make_row_label(kicker: str, name: str, y: float) -> tuple[VGroup, Text, Text]:
    k = Text(kicker, font=MONO, font_size=17, color=MUTED)
    n = Text(name, font=SANS, font_size=29, color=TXT)
    group = VGroup(k, n).arrange(DOWN, aligned_edge=LEFT, buff=0.07)
    group.move_to([-6.35, y, 0], aligned_edge=LEFT)
    return group, k, n


def make_check() -> VGroup:
    circle = Circle(
        radius=0.16,
        stroke_color=OUT_C,
        stroke_width=1.4,
        fill_color=OUT_C,
        fill_opacity=0.10,
    )
    tick = VGroup(
        Line([-0.065, 0.00, 0], [-0.015, -0.050, 0], stroke_color=OUT_C, stroke_width=2.2),
        Line([-0.015, -0.050, 0], [0.075, 0.060, 0], stroke_color=OUT_C, stroke_width=2.2),
    )
    return VGroup(circle, tick)


def dot_grid() -> VGroup:
    dots = VGroup()
    for x in np.arange(-6.90, 6.91, 0.72):
        for y in np.arange(-3.75, 3.76, 0.72):
            dots.add(
                Dot(
                    point=[x, y, 0],
                    radius=0.0085,
                    color=GRID_C,
                    fill_opacity=0.55,
                    stroke_width=0,
                )
            )
    return dots


# --------------------------------------------------------------------------- #
# Scene
# --------------------------------------------------------------------------- #


class SiphonExplainer(Scene):
    def construct(self):
        self.camera.background_color = BG
        self.add(dot_grid())

        # ------------------------------------------------------------- header
        wordmark = Text("SIPHON", font=SANS, weight=BOLD, font_size=78, color=TXT)
        wordmark.move_to([-5.98, 3.18, 0], aligned_edge=LEFT)

        accent = Rectangle(
            width=0.055,
            height=wordmark.height * 0.95,
            fill_color=SRC_C,
            fill_opacity=1,
            stroke_width=0,
        )
        accent.next_to(wordmark, LEFT, buff=0.20)

        subtitle = Text(
            "in-place conversion · no 2× copy",
            font=SANS,
            font_size=24,
            color=MUTED,
        )
        subtitle.next_to(wordmark, DOWN, buff=0.09, aligned_edge=LEFT)

        chips = VGroup(
            make_pill("MKV → MP4"),
            make_pill("≈ 1× DISK", color=OUT_C, border="#1E6B5E", fill=0.06),
        ).arrange(RIGHT, buff=0.12)
        chips.move_to([6.15, 3.18, 0], aligned_edge=RIGHT)

        header_rule = Line(
            [-6.35, 2.26, 0],
            [6.15, 2.26, 0],
            stroke_color=LINE_C,
            stroke_width=1,
            stroke_opacity=0.9,
        )

        header = VGroup(accent, wordmark, subtitle, chips, header_rule)

        # --------------------------------------------------------------- rows
        src_labels, _, src_name = make_row_label("SOURCE", "film.mkv", SRC_Y)
        out_labels, _, _ = make_row_label("OUTPUT", "film.mp4", OUT_Y)

        source_chunks: list[VGroup] = []
        source_rects: list[RoundedRectangle] = []
        for i in range(N_CHUNKS):
            chunk, rect = make_chunk(i, SRC_Y, source=True)
            source_chunks.append(chunk)
            source_rects.append(rect)

        slots = [make_slot(i) for i in range(N_CHUNKS)]
        output_chunks = [make_chunk(i, OUT_Y, source=False)[0] for i in range(N_CHUNKS)]

        # The cut marker rides the shrinking tail of the source.
        blade = VGroup(
            Line(
                [0, -0.42, 0],
                [0, 0.42, 0],
                stroke_color=SRC_EDGE,
                stroke_width=7,
                stroke_opacity=0.10,
            ),
            Line(
                [0, -0.40, 0],
                [0, 0.40, 0],
                stroke_color="#FFF4E2",
                stroke_width=1.6,
                stroke_opacity=0.70,
            ),
        ).move_to([BAR_RIGHT, SRC_Y, 0])

        # ------------------------------------------------------------- footer
        footer_rule = Line(
            [-6.35, -2.48, 0],
            [6.15, -2.48, 0],
            stroke_color=LINE_C,
            stroke_width=1,
            stroke_opacity=0.7,
        )

        tick_boxes = VGroup(
            *[
                RoundedRectangle(
                    width=0.15,
                    height=0.15,
                    corner_radius=0.03,
                    stroke_color=SLOT_C,
                    stroke_width=1.2,
                    fill_color=OUT_C,
                    fill_opacity=0.0,
                )
                for _ in range(N_CHUNKS)
            ]
        ).arrange(RIGHT, buff=0.07)

        journal = (
            VGroup(
                Text("JOURNAL", font=MONO, font_size=15, color=DIM),
                tick_boxes,
            )
            .arrange(RIGHT, buff=0.20)
            .move_to([-6.35, -2.95, 0], aligned_edge=LEFT)
        )

        caption = Text(
            "tail-first · keyframe-aligned chunks",
            font=MONO,
            font_size=16,
            color=MUTED,
        )
        caption.move_to([6.15, -2.95, 0], aligned_edge=RIGHT)

        # ---------------------------------------------------------- entrance
        self.play(FadeIn(header, shift=UP * 0.10), run_time=0.50)

        self.play(
            LaggedStart(
                *[FadeIn(c, shift=UP * 0.05) for c in source_chunks],
                lag_ratio=0.08,
            ),
            LaggedStart(*[FadeIn(s) for s in slots], lag_ratio=0.08),
            FadeIn(src_labels),
            FadeIn(out_labels),
            FadeIn(journal),
            FadeIn(caption),
            FadeIn(footer_rule),
            FadeIn(blade),
            run_time=0.50,
        )
        self.wait(0.08)

        # ---------------------------------------------------- tail-first loop
        # Process chunk 4 → chunk 0. Each cycle:
        #   highlight → copy down into its final output offset → verify/checkpoint
        #   → fade the source chunk and advance the cut marker.
        for order, i in enumerate(range(N_CHUNKS - 1, -1, -1)):
            source_chunk = source_chunks[i]
            source_rect = source_rects[i]
            center_x = CHUNK_CENTERS[i]
            out_center = np.array([center_x, OUT_Y, 0.0])

            self.play(
                source_rect.animate.set_fill(SRC_HI, opacity=1.0),
                run_time=0.08,
            )

            flying = make_chunk(i, OUT_Y, source=False)[0]
            flying.move_to([center_x, SRC_Y, 0])
            flying.set_opacity(0)
            self.play(
                flying.animate.set_opacity(1).move_to(out_center),
                run_time=0.18,
                rate_func=smooth,
            )

            self.remove(flying, slots[i])
            self.add(output_chunks[i])

            tick = tick_boxes[order]
            self.play(
                Flash(
                    out_center,
                    color=OUT_EDGE,
                    flash_radius=0.42,
                    line_length=0.13,
                    num_lines=10,
                ),
                tick.animate.set_fill(OUT_C, opacity=0.9).set_stroke(OUT_C, width=1.2),
                run_time=0.15,
            )

            cut_x = center_x - CHUNK_W / 2
            self.play(
                source_chunk.animate.set_opacity(0.0),
                blade.animate.move_to([cut_x, SRC_Y, 0]),
                run_time=0.16,
            )
            self.remove(source_chunk)
            self.wait(0.07)

        # ----------------------------------------------------------- finalise
        strike = Line(
            src_name.get_left() + LEFT * 0.08 + DOWN * 0.02,
            src_name.get_right() + RIGHT * 0.08 + DOWN * 0.02,
            stroke_color=MUTED,
            stroke_width=1.6,
            stroke_opacity=0.9,
        )

        final_outline = RoundedRectangle(
            width=BAR_W + 0.04,
            height=BAR_H + 0.04,
            corner_radius=0.09,
            stroke_color=OUT_C,
            stroke_width=2.0,
            fill_opacity=0,
        ).move_to([(BAR_LEFT + BAR_RIGHT) / 2, OUT_Y, 0])

        check = make_check().move_to([6.50, OUT_Y, 0])

        self.play(
            src_labels.animate.set_opacity(0.32),
            Create(strike),
            Create(final_outline),
            FadeIn(check, scale=0.6),
            FadeOut(blade),
            run_time=0.55,
        )

        # ------------------------------------------------------------- payoff
        payoff_prefix = Text("peak extra space:", font=SANS, font_size=36, color=TXT)
        payoff_hi = Text("one chunk", font=SANS, font_size=36, weight=BOLD, color=OUT_C)
        payoff = VGroup(payoff_prefix, payoff_hi).arrange(
            RIGHT, buff=0.12, aligned_edge=DOWN
        )
        payoff.move_to([0, -1.72, 0])

        underline = Line(
            payoff_hi.get_corner(DL) + DOWN * 0.07,
            payoff_hi.get_corner(DR) + DOWN * 0.07,
            stroke_color=OUT_C,
            stroke_width=2,
            stroke_opacity=0.7,
        )

        self.play(
            FadeIn(payoff, shift=UP * 0.10),
            Create(underline),
            run_time=0.47,
        )

        self.wait(0.70)
