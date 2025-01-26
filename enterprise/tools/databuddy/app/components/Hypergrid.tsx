import hypergrid, { registerTheme } from "fin-hypergrid";
import React from "react";

export type HypergridProps = {
  data: Record<string, any>[];
};

const FONT_FAMILY = '"Noto Sans"';

// registerTheme({
//   themeName: "databuddy",
//   columnHeaderBackgroundColor: "rgb(215, 220, 227)",
//   rowHeaderBackgroundColor: "rgb(215, 220, 227)",
//   topLeftBackgroundColor: "rgb(215, 220, 227)",
//   lineColor: "rgb(176, 181, 186)",
//   backgroundColor2: "rgb(255, 255, 255)",
//   color: "rgb(25, 25, 25)",
//   backgroundColor: "#fff",
//   columnHeaderColor: "#000",
//   rowHeaderColor: "#000",
//   topLeftColor: "#000",
//   backgroundSelectionColor: "#fff",
//   foregroundSelectionColor: "#000",
//   columnHeaderForegroundSelectionColor: "#000",
//   columnHeaderBackgroundSelectionColor: "rgb(176, 181, 186)",
//   rowHeaderForegroundSelectionColor: "rgb(215, 220, 227)",
//   fixedColumnBackgroundSelectionColor: "rgb(176, 181, 186)",
//   fontFamily: FONT_FAMILY,
//   foregroundSelectionFont: FONT_FAMILY,
//   columnHeaderForegroundSelectionFont: FONT_FAMILY,
//   rowHeaderForegroundSelectionFont: FONT_FAMILY,
//   treeHeaderForegroundSelectionFont: FONT_FAMILY,
// });

registerTheme({
  themeName: "classical",
  columnHeaderBackgroundColor: "rgb(96, 101, 101)",
  rowHeaderBackgroundColor: "rgb(96, 101, 101)",
  topLeftBackgroundColor: "rgb(96, 101, 101)",
  lineColor: "rgb(196, 199, 199)",
  backgroundColor2: "rgb(236, 236, 236)",
  color: "rgb(89, 79, 79)",
  fontFamily: "Ledger, serif",
  backgroundColor: "rgb(228, 231, 231)",
  columnHeaderColor: "rgb(96, 101, 101)",
  rowHeaderColor: "rgb(96, 101, 101)",
  topLeftColor: "rgb(96, 101, 101)",
  backgroundSelectionColor: "rgb(196, 199, 199)",
  foregroundSelectionColor: "rgb(96, 101, 101)",
  columnHeaderForegroundSelectionColor: "rgb(96, 101, 101)",
  columnHeaderBackgroundSelectionColor: "rgb(196, 199, 199)",
  rowHeaderForegroundSelectionColor: "rgb(96, 101, 101)",
  fixedColumnBackgroundSelectionColor: "rgb(196, 199, 199)",
});

export default function Hypergrid({ data }: HypergridProps) {
  const containerRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    const grid = new hypergrid(containerRef.current!, { data: data });
    // grid.theme = "databuddy";
    grid.theme = "classical";
    return () => {
      grid.terminate();
    };
  }, []);

  return <div ref={containerRef} />;
}
