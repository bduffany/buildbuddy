declare module "fin-hypergrid" {
  export default class Hypergrid<R extends RowData = RowData> {
    behavior: Behavior | null;

    constructor(container: HTMLElement, options?: GridOptions<R>);

    applyTheme(theme: string);
    getTheme(): string;
    set theme(theme: string);
    get theme(): string;

    setData(data: R[]): void;

    terminate(): void;
  }

  export type RowData = Record<string, any>;

  export type GridOptions<R extends RowData = RowData> = {
    data?: R[];
  };

  export type Theme = {
    columnHeaderBackgroundColor?: string;
    rowHeaderBackgroundColor?: string;
    topLeftBackgroundColor?: string;
    lineColor?: string;
    backgroundColor2?: string;
    color?: string;
    fontFamily?: string;
    backgroundColor?: string;
    columnHeaderColor?: string;
    rowHeaderColor?: string;
    topLeftColor?: string;
    backgroundSelectionColor?: string;
    foregroundSelectionColor?: string;
    columnHeaderForegroundSelectionColor?: string;
    columnHeaderBackgroundSelectionColor?: string;
    rowHeaderForegroundSelectionColor?: string;
    fixedColumnBackgroundSelectionColor?: string;
    columnHeaderForegroundSelectionFont?: string;
    rowHeaderForegroundSelectionFont?: string;
    treeHeaderForegroundSelectionFont?: string;
    foregroundSelectionFont?: string;
  };

  export function registerTheme(name: string, theme?: Theme);
  export function registerTheme(theme: Theme & { themeName: string });
}
