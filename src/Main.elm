module Main exposing (main)

import Browser
import Browser.Dom exposing (Error, Viewport)
import Browser.Events
import CustomSvg
import Element exposing (Element)
import Element.Background as Background
import Element.Border as Border
import Element.Input as Input
import Html exposing (Html)
import Http
import Maybe.Extra exposing (isJust)
import Process
import String
import Svg exposing (svg)
import Svg.Attributes as Attributes
import Task
import Types exposing (State)


type alias Model =
    Result String OkModel


type alias OkModel =
    { svgViewport : Maybe Viewport
    , state : State
    }


initialModel : Model
initialModel =
    Ok
        { svgViewport = Nothing
        , state = Types.initialState
        }


type Msg
    = GetSvgViewport (Result Error Viewport)
    | ResizeWindow Int Int
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


update : Msg -> Model -> ( Model, Cmd Msg )
update msg modelRes =
    case modelRes of
        Err _ ->
            ( modelRes, Cmd.none )

        Ok model ->
            case msg of
                GetSvgViewport result ->
                    case result of
                        Ok viewport ->
                            ( Ok { model | svgViewport = Just viewport }, Cmd.none )

                        Err _ ->
                            ( Err "error getting viewport", Cmd.none )

                ResizeWindow _ _ ->
                    ( Ok model, Task.attempt GetSvgViewport Browser.Dom.getViewport )

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
    Element.layout [] <|
        Element.row
            [ Element.width Element.fill
            , Element.height Element.fill
            ]
            [ leftNav model
            , Element.html <| viewSvg model
            , rightNav model
            ]


leftNav : Model -> Element Msg
leftNav modelRes =
    case modelRes of
        Err _ ->
            Element.none

        Ok model ->
            navColumn <|
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
            navColumn
                [ navColumnItem
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


navColumn : List (Element msg) -> Element msg
navColumn =
    Element.column
        [ Element.width <| Element.px 200
        , Element.height Element.fill
        , Border.color <| Element.rgb255 0 0 0
        , Border.solid
        , Border.width 2
        , Background.color <| Element.rgb255 128 128 128
        , Element.padding 4
        , Element.spacing 4
        ]


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


viewSvg : Model -> Html Msg
viewSvg modelRes =
    case modelRes of
        Err err ->
            Html.text err

        Ok model ->
            let
                width =
                    model.svgViewport
                        |> Maybe.map (.viewport >> .width >> round)
                        |> Maybe.withDefault 100

                height =
                    model.svgViewport
                        |> Maybe.map (.viewport >> .height >> round)
                        |> Maybe.withDefault 100

                inactiveFactories =
                    List.map
                        (CustomSvg.drawFactory model.state.ymin model.state.ymax)
                        (List.filter (not << .active) model.state.factories)

                profitableFactories =
                    List.map (CustomSvg.drawFactory model.state.ymin model.state.ymax)
                        (List.filter (.profitability >> (<) 0) (List.filter .active model.state.factories))

                unprofitableFactories =
                    List.map (CustomSvg.drawFactory model.state.ymin model.state.ymax)
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
            in
            svg
                [ Attributes.width <| String.fromInt width
                , Attributes.height <| String.fromInt height
                , Attributes.viewBox <|
                    String.join " "
                        [ String.fromInt model.state.xmin
                        , String.fromInt model.state.ymin
                        , String.fromInt (model.state.xmax - model.state.xmin)
                        , String.fromInt (model.state.ymax - model.state.ymin)
                        ]
                ]
                [ inactiveFactoryCircles
                , Svg.g [] (List.map (CustomSvg.drawTransport model.state.ymin model.state.ymax) model.state.transports)
                , unprofitableFactoryLabels
                , profitableFactoryLabels
                , unprofitableFactoryCircles
                , profitableFactoryCircles
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
                    [ Task.attempt GetSvgViewport Browser.Dom.getViewport
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


sleepAndPoll : Cmd Msg
sleepAndPoll =
    Process.sleep 50
        |> Task.andThen (\_ -> Task.succeed GetState)
        |> Task.perform identity


subscriptions : Model -> Sub Msg
subscriptions _ =
    Browser.Events.onResize ResizeWindow
