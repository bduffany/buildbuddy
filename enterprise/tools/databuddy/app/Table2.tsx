import React from "react";
import Hypergrid from "./components/Hypergrid";

export type TableProps = {
  header: string[];
  rows: any[][];
  headerTooltips: string[];

  sort: string | null | undefined;
  sortDir: "asc" | "desc" | null | undefined;
  onSortChange: (sort: string | undefined, sortDir: "asc" | "desc" | undefined) => void;
};

export default function Table2({ header, rows, headerTooltips, sort, sortDir, onSortChange }: TableProps) {
  const data = React.useMemo(() => {
    return rows.map((row) => {
      return Object.fromEntries(header.map((header, i) => [header, row[i]]));
    });
  }, [header, rows]);

  return <Hypergrid data={data} />;
}
