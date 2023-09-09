module Types exposing (..)

import Json.Decode as Decode exposing (Decoder)


type alias State =
    { factories : List Factory
    , transports : List Transport
    , tick : Int
    , running : Bool
    , xmin : Int
    , xmax : Int
    , ymin : Int
    , ymax : Int
    }


type alias Factory =
    { location : Location
    , recipe : String
    , products : List String
    , profitability : Float
    , active : Bool
    }


type alias Transport =
    { origin : Location
    , destination : Location
    , rate : Float
    }


type alias Location =
    { x : Int
    , y : Int
    }


initialState : State
initialState =
    { factories = []
    , transports = []
    , tick = 0
    , running = False
    , xmin = -100
    , xmax = 100
    , ymin = -100
    , ymax = 100
    }



-- JSON decoders


locationDecoder : Decoder Location
locationDecoder =
    Decode.map2 Location
        (Decode.field "x" Decode.int)
        (Decode.field "y" Decode.int)


transportDecoder : Decoder Transport
transportDecoder =
    Decode.map3 Transport
        (Decode.field "origin" locationDecoder)
        (Decode.field "destination" locationDecoder)
        (Decode.field "rate" Decode.float)


factoryDecoder : Decoder Factory
factoryDecoder =
    Decode.map5 Factory
        (Decode.field "location" locationDecoder)
        (Decode.field "recipe" Decode.string)
        (Decode.field "products" (Decode.list Decode.string))
        (Decode.field "profitability" Decode.float)
        (Decode.field "active" Decode.bool)


stateDecoder : Decoder State
stateDecoder =
    Decode.map8 State
        (Decode.field "factories" (Decode.list factoryDecoder))
        (Decode.field "transports" (Decode.list transportDecoder))
        (Decode.field "tick" Decode.int)
        (Decode.field "running" Decode.bool)
        (Decode.field "xmin" Decode.int)
        (Decode.field "xmax" Decode.int)
        (Decode.field "ymin" Decode.int)
        (Decode.field "ymax" Decode.int)
