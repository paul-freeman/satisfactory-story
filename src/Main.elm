module Main exposing (main)

import Browser
import Browser.Dom exposing (Error, Viewport)
import Browser.Events
import CustomSvg
import Dict exposing (Dict)
import Element exposing (Element, scrollbarY)
import Element.Background as Background
import Element.Border as Border
import Element.Font as Font
import Element.Input as Input
import Html exposing (Html)
import Html.Events.Extra.Pointer as Pointer
import Html.Events.Extra.Wheel as Wheel
import Http
import Json.Decode as Decode
import Maybe.Extra exposing (isJust, isNothing)
import Process
import String
import Svg exposing (svg)
import Svg.Attributes as Attributes
import Task
import Types exposing (State)


type alias Model =
    Result String OkModel


type alias OkModel =
    { viewport : Viewport
    , viewboxOffset : { x : Float, y : Float }
    , svg : { viewbox : String }
    , draggingState : Maybe DraggingState
    , zoom : Float
    , recipes : List Types.Recipe
    , activeRecipes : Dict String Bool
    , state : State
    }


type alias DraggingState =
    { downX : Float, downY : Float, tempOffsetX : Float, tempOffsetY : Float }


initialModel : Model
initialModel =
    Ok
        { viewport = defaultViewport
        , viewboxOffset = { x = 0, y = 0 }
        , svg = { viewbox = "0 0 100 100" }
        , draggingState = Nothing
        , zoom = 1
        , recipes = []
        , activeRecipes = Dict.empty
        , state = Types.initialState
        }


defaultViewport : Viewport
defaultViewport =
    { viewport =
        { x = 0
        , y = 0
        , width = 100
        , height = 100
        }
    , scene =
        { width = 100
        , height = 100
        }
    }


type Msg
    = GetViewport (Result Error Viewport)
    | ResizeWindow Int Int
    | Zooming Wheel.Event
    | DownPointer Pointer.Event
    | MovePointer Pointer.Event
    | UpPointer Pointer.Event
    | GetState
    | StateResult (Result Http.Error State)
    | GetTick
    | TickResult (Result Http.Error State)
    | GetRun
    | RunResult (Result Http.Error ())
    | GetStop
    | StopResult (Result Http.Error State)
    | GetReset
    | ResetResult (Result Http.Error State)
    | GetRecipes
    | RecipeResult (Result Http.Error (List Types.Recipe))
    | SetRecipe String Bool


update : Msg -> Model -> ( Model, Cmd Msg )
update msg modelRes =
    case modelRes of
        Err _ ->
            ( modelRes, Cmd.none )

        Ok model ->
            case msg of
                GetViewport result ->
                    case result of
                        Ok viewport ->
                            ( Ok { model | viewport = viewport }, Cmd.none )

                        Err _ ->
                            ( Err "error getting viewport", Cmd.none )

                ResizeWindow _ _ ->
                    ( Ok model, Task.attempt GetViewport Browser.Dom.getViewport )

                Zooming event ->
                    let
                        newZoom =
                            model.zoom + (event.deltaY / 1000.0)
                    in
                    ( Ok { model | zoom = newZoom }, Cmd.none )

                DownPointer event ->
                    ( Ok
                        { model
                            | draggingState =
                                Just
                                    { downX = Tuple.first event.pointer.offsetPos
                                    , downY = Tuple.second event.pointer.offsetPos
                                    , tempOffsetX = 0
                                    , tempOffsetY = 0
                                    }
                        }
                    , Cmd.none
                    )

                MovePointer event ->
                    case model.draggingState of
                        Nothing ->
                            ( Ok model, Cmd.none )

                        Just draggingState ->
                            ( Ok
                                { model
                                    | draggingState =
                                        Just
                                            { draggingState
                                                | downX = Tuple.first event.pointer.offsetPos
                                                , downY = Tuple.second event.pointer.offsetPos
                                                , tempOffsetX = draggingState.tempOffsetX + (draggingState.downX - Tuple.first event.pointer.offsetPos)
                                                , tempOffsetY = draggingState.tempOffsetY + (draggingState.downY - Tuple.second event.pointer.offsetPos)
                                            }
                                }
                            , Cmd.none
                            )

                UpPointer event ->
                    case model.draggingState of
                        Nothing ->
                            ( Ok model, Cmd.none )

                        Just draggingState ->
                            ( Ok
                                { model
                                    | viewboxOffset =
                                        { x = model.viewboxOffset.x + panOffsetMultiplier model.zoom * (draggingState.tempOffsetX + (draggingState.downX - Tuple.first event.pointer.offsetPos))
                                        , y = model.viewboxOffset.y + panOffsetMultiplier model.zoom * (draggingState.tempOffsetY + (draggingState.downY - Tuple.second event.pointer.offsetPos))
                                        }
                                    , draggingState = Nothing
                                }
                            , Cmd.none
                            )

                GetState ->
                    ( Ok model, getStateCmd )

                StateResult result ->
                    stateResult result model

                GetTick ->
                    ( Ok model, getTickCmd )

                TickResult result ->
                    stateResult result model

                GetRun ->
                    ( Ok model, getRunCmd )

                RunResult result ->
                    case result of
                        Ok _ ->
                            ( Ok model, getStateCmd )

                        Err _ ->
                            ( Err "error running", Cmd.none )

                GetStop ->
                    ( Ok model, getStopCmd )

                StopResult result ->
                    stateResult result model

                GetReset ->
                    ( Ok model, getResetCmd )

                ResetResult result ->
                    stateResult result model

                GetRecipes ->
                    ( Ok model, getRecipesCmd )

                RecipeResult result ->
                    case result of
                        Ok recipes ->
                            ( Ok { model | recipes = recipes, activeRecipes = Dict.fromList (List.map (\r -> ( r.name, True )) recipes) }, Cmd.none )

                        Err _ ->
                            ( Err "error fetching recipes", Cmd.none )

                SetRecipe name set ->
                    ( Ok { model | activeRecipes = Dict.insert name set model.activeRecipes }, Cmd.none )


stateResult : Result error State -> OkModel -> ( Result String OkModel, Cmd Msg )
stateResult result model =
    case result of
        Ok newState ->
            if newState.running then
                ( Ok { model | state = newState }, sleepAndPoll )

            else
                ( Ok { model | state = newState }, Cmd.none )

        Err _ ->
            ( Err "error fetching state", Cmd.none )


view : Model -> Html Msg
view model =
    Element.layout
        []
        (Element.row
            [ Element.width Element.fill
            , Element.height Element.fill
            ]
            [ leftNav model
            , Element.column []
                [ Element.html <| viewSvg model
                ]
            , rightNav model
            ]
        )


leftNav : Model -> Element Msg
leftNav modelRes =
    case modelRes of
        Err _ ->
            Element.none

        Ok model ->
            navColumn modelRes <|
                navColumnItem
                    [ Element.text "Tick"
                    , Element.text <| String.fromInt model.state.tick
                    ]
                    :: runStopButton model


runStopButton : OkModel -> List (Element Msg)
runStopButton model =
    if model.state.running then
        [ navColumnButton (Just GetStop) "Stop" ]

    else
        [ navColumnButton (Just GetRun) "Run"
        , navColumnButton (Just GetTick) "Tick"
        , navColumnButton (Just GetReset) "Reset"
        ]


rightNav : Model -> Element Msg
rightNav modelRes =
    case modelRes of
        Err _ ->
            Element.none

        Ok model ->
            navColumn modelRes
                ([ navColumnItem
                    [ Element.text "Producers"
                    , Element.text <| String.fromInt <| List.length model.state.factories
                    ]
                 , navColumnItem
                    [ Element.text "Transports"
                    , Element.text <| String.fromInt <| List.length model.state.transports
                    ]
                 , navColumnItem
                    [ Element.text "Inactive"
                    , Element.text <| String.fromInt <| List.length <| List.filter (not << .active) model.state.factories
                    ]
                 ]
                    ++ (model.recipes
                            |> List.filter (.name >> String.startsWith "Alternate")
                            |> List.sortBy .name
                            |> List.map (\r -> recipeCheckbox r model)
                       )
                    ++ (model.recipes
                            |> List.filter (.name >> String.startsWith "Alternate" >> not)
                            |> List.sortBy .name
                            |> List.map (\r -> recipeCheckbox r model)
                       )
                )


recipeCheckbox : Types.Recipe -> OkModel -> Element Msg
recipeCheckbox recipe model =
    navColumnItem
        [ Input.checkbox
            [ Element.width Element.fill
            , Element.height Element.fill
            ]
            { onChange = SetRecipe recipe.name
            , icon = Input.defaultCheckbox
            , checked = Dict.get recipe.name model.activeRecipes |> Maybe.withDefault False
            , label = Input.labelRight [ Font.size 11 ] <| Element.text recipe.name
            }
        ]


navColumn : Model -> List (Element msg) -> Element msg
navColumn model =
    Element.column
        [ Element.width <| Element.px navColumnWidth
        , Element.height
            (Result.map (.viewport >> .viewport >> .height) model
                |> Result.withDefault 0
                |> round
                |> Element.px
            )
        , Border.color <| Element.rgb255 0 0 0
        , Border.solid
        , Border.width 2
        , Background.color <| Element.rgb255 128 128 128
        , Element.padding 4
        , Element.spacing 4
        , scrollbarY
        ]


navColumnWidth : Int
navColumnWidth =
    200


navColumnItem : List (Element msg) -> Element msg
navColumnItem =
    Element.column
        [ Element.width Element.fill
        , Background.color <| Element.rgb255 255 255 255
        , Element.padding 4
        , Border.color <| Element.rgb255 0 0 0
        , Border.solid
        , Border.width 1
        ]


navColumnButton : Maybe msg -> String -> Element msg
navColumnButton onPress text =
    navColumnItem <|
        [ Input.button
            [ Element.width Element.fill
            , Element.height Element.fill
            , Background.color <| Element.rgb255 192 192 192
            ]
            { onPress = onPress
            , label = Element.text text
            }
        ]


panOffsetMultiplier : Float -> Float
panOffsetMultiplier zoom =
    160 / zoom


viewSvg : Model -> Html Msg
viewSvg modelRes =
    case modelRes of
        Err err ->
            Html.text err

        Ok model ->
            let
                width =
                    round model.viewport.viewport.width - (2 * navColumnWidth)

                height =
                    round model.viewport.viewport.height

                r =
                    String.fromFloat (900 / model.zoom)

                fontSize =
                    String.fromFloat (2000 / model.zoom)

                inactiveFactories =
                    List.map
                        (CustomSvg.drawFactory model.state.ymin model.state.ymax r fontSize)
                        (List.filter (not << .active) model.state.factories)

                profitableFactories =
                    List.map (CustomSvg.drawFactory model.state.ymin model.state.ymax r fontSize)
                        (List.filter (.profitability >> (<) 0) (List.filter .active model.state.factories))

                unprofitableFactories =
                    List.map (CustomSvg.drawFactory model.state.ymin model.state.ymax r fontSize)
                        (List.filter (.profitability >> (>=) 0) (List.filter .active model.state.factories))

                inactiveFactoryCircles =
                    Svg.g [] <| List.map .circle inactiveFactories

                profitableFactoryCircles =
                    Svg.g [] <| List.map .circle profitableFactories

                unprofitableFactoryCircles =
                    Svg.g [] <| List.map .circle unprofitableFactories

                profitableFactoryLabels =
                    List.map .text profitableFactories
                        |> List.filter isJust
                        |> List.map (Maybe.withDefault (Svg.text ""))
                        |> Svg.g []

                unprofitableFactoryLabels =
                    List.map .text unprofitableFactories
                        |> List.filter isJust
                        |> List.map (Maybe.withDefault (Svg.text ""))
                        |> Svg.g []

                strokeWidth =
                    String.fromFloat (200 / model.zoom)
            in
            svg
                ([ Attributes.width <| String.fromInt width
                 , Attributes.height <| String.fromInt height
                 , Attributes.viewBox <|
                    calculateSvgViewbox
                        { offsetX = model.viewboxOffset.x
                        , offsetY = model.viewboxOffset.y
                        , draggingState = model.draggingState
                        , zoom = model.zoom
                        , state = model.state
                        }
                 , Wheel.onWheel Zooming
                 , Pointer.onDown DownPointer
                 , Pointer.onUp UpPointer
                 ]
                    ++ (if isNothing model.draggingState then
                            []

                        else
                            [ Pointer.onMove MovePointer ]
                       )
                )
                [ inactiveFactoryCircles
                , Svg.g [] (List.map (CustomSvg.drawTransport model.state.ymin model.state.ymax strokeWidth) model.state.transports)
                , unprofitableFactoryLabels
                , profitableFactoryLabels
                , unprofitableFactoryCircles
                , profitableFactoryCircles
                ]


calculateSvgViewbox :
    { offsetX : Float
    , offsetY : Float
    , draggingState : Maybe DraggingState
    , zoom : Float
    , state : State
    }
    -> String
calculateSvgViewbox { offsetX, offsetY, draggingState, zoom, state } =
    let
        tempOffsetX =
            draggingState
                |> Maybe.map .tempOffsetX
                |> Maybe.withDefault 0

        tempOffsetY =
            draggingState
                |> Maybe.map .tempOffsetY
                |> Maybe.withDefault 0

        newOffsetX =
            offsetX + tempOffsetX * panOffsetMultiplier zoom

        newOffsetY =
            offsetY + tempOffsetY * panOffsetMultiplier zoom
    in
    String.join " "
        [ String.fromFloat (toFloat state.xmin + newOffsetX)
        , String.fromFloat (toFloat state.ymin + newOffsetY)
        , String.fromFloat (toFloat (state.xmax - state.xmin) / zoom)
        , String.fromFloat (toFloat (state.ymax - state.ymin) / zoom)
        ]


type alias Flags =
    ()


main : Program Flags Model Msg
main =
    Browser.element
        { init =
            \_ ->
                ( initialModel
                , Cmd.batch
                    [ Task.attempt GetViewport Browser.Dom.getViewport
                    , getRecipesCmd
                    , getStateCmd
                    ]
                )
        , subscriptions = subscriptions
        , view = view
        , update = update
        }


getStateCmd : Cmd Msg
getStateCmd =
    Http.get
        { url = "http://localhost:28100/state"
        , expect = Http.expectJson StateResult Types.stateDecoder
        }


getTickCmd : Cmd Msg
getTickCmd =
    Http.get
        { url = "http://localhost:28100/tick"
        , expect = Http.expectJson StateResult Types.stateDecoder
        }


getRunCmd : Cmd Msg
getRunCmd =
    Http.get
        { url = "http://localhost:28100/run"
        , expect = Http.expectWhatever RunResult
        }


getStopCmd : Cmd Msg
getStopCmd =
    Http.get
        { url = "http://localhost:28100/stop"
        , expect = Http.expectJson StateResult Types.stateDecoder
        }


getResetCmd : Cmd Msg
getResetCmd =
    Http.get
        { url = "http://localhost:28100/reset"
        , expect = Http.expectJson StateResult Types.stateDecoder
        }


getRecipesCmd : Cmd Msg
getRecipesCmd =
    Http.get
        { url = "http://localhost:28100/recipes"
        , expect = Http.expectJson RecipeResult (Decode.list Types.recipeDecoder)
        }


sleepAndPoll : Cmd Msg
sleepAndPoll =
    Process.sleep 50
        |> Task.andThen (\_ -> Task.succeed GetState)
        |> Task.perform identity


subscriptions : Model -> Sub Msg
subscriptions _ =
    Browser.Events.onResize ResizeWindow
