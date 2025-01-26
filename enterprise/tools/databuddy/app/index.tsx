import React from "react";
import { RouterProvider, useRoute } from "react-router5";
import Home from "./Home";
import Query from "./Query";
import router from "./router";
import { Toast } from "./toast";
import ReactDOM from "react-dom";

export default function App() {
  return (
    <RouterProvider router={router}>
      <Router />
      <Toast />
    </RouterProvider>
  );
}

const VIEWS: Record<string, React.FC> = {
  home: Home,
  query: Query,
};

function Router() {
  const { route } = useRoute();
  const ComponentForRoute = VIEWS[route?.name] || VIEWS.home;
  return <ComponentForRoute />;
}

ReactDOM.render(<App />, document.querySelector("#root")!);
